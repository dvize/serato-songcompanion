const { app, BrowserWindow, ipcMain, nativeImage, globalShortcut, Tray, screen } = require('electron')
const path = require('path')
const fs = require('fs')
const { spawn } = require('child_process')

let mainWindow
let backendProcess
let tray = null

// Defaults — overridden by backend settings once the Go server is up
let collapsedSize = { width: 340, height: 64 }
let expandedSize = { width: 860, height: 720 }

// ── Window state persistence ───────────────────────────────────────────────

const statePath = () => path.join(app.getPath('userData'), 'window-state.json')
let winState = { x: null, y: null, expandedW: 860, expandedH: 720 }

function loadWindowState() {
    try {
        const data = JSON.parse(fs.readFileSync(statePath(), 'utf8'))
        if (Number.isFinite(data.x)) winState.x = data.x
        if (Number.isFinite(data.y)) winState.y = data.y
        if (Number.isFinite(data.expandedW)) winState.expandedW = data.expandedW
        if (Number.isFinite(data.expandedH)) winState.expandedH = data.expandedH
    } catch (_) {}
    expandedSize = { width: winState.expandedW, height: winState.expandedH }
}

let saveStateTimer = null
function scheduleSaveWindowState() {
    clearTimeout(saveStateTimer)
    saveStateTimer = setTimeout(saveWindowState, 500)
}

function saveWindowState() {
    if (!mainWindow || mainWindow.isDestroyed()) return
    const [x, y] = mainWindow.getPosition()
    const [w, h] = mainWindow.getSize()
    if (w > collapsedSize.width || h > collapsedSize.height) {
        winState.expandedW = w
        winState.expandedH = h
        expandedSize = { width: w, height: h }
    }
    if (mainWindow.isVisible() && !mainWindow.isMinimized()) {
        winState.x = x
        winState.y = y
    }
    try {
        fs.mkdirSync(path.dirname(statePath()), { recursive: true })
        fs.writeFileSync(statePath(), JSON.stringify(winState))
    } catch (err) {
        console.error('Failed to save window state:', err.message)
    }
}

// Clamp a rect into the work area of the display nearest to it, so the window
// is never spawned partially off-screen (multi-monitor aware).
function clampRect(x, y, width, height) {
    const display = screen.getDisplayNearestPoint({ x: x || 0, y: y || 0 })
    const wa = display.workArea
    const nx = Math.min(Math.max(wa.x, x), wa.x + wa.width - width)
    const ny = Math.min(Math.max(wa.y, y), wa.y + wa.height - height)
    return { x: nx, y: ny, width, height }
}

// ── Paths ──────────────────────────────────────────────────────────────────

function backendBinaryName() {
    if (process.platform === 'win32') return 'companion.exe'
    if (process.platform === 'linux') {
        return process.arch === 'arm64' ? 'companion-linux-arm64' : 'companion-linux-amd64'
    }
    return process.arch === 'arm64' ? 'companion-mac-arm64' : 'companion-mac-amd64'
}

function backendPath() {
    return app.isPackaged
        ? path.join(process.resourcesPath, backendBinaryName())
        : path.join(__dirname, '..', backendBinaryName())
}

function backendCwd() {
    return app.isPackaged
        ? process.resourcesPath
        : path.join(__dirname, '..')
}

// ── Backend ────────────────────────────────────────────────────────────────

function startBackend() {
    const exe = backendPath()
    const cwd = backendCwd()

    // Ensure executable bit is set on macOS/Linux (may be lost during packaging)
    if (process.platform !== 'win32') {
        try { fs.chmodSync(exe, 0o755) } catch (_) {}
    }

    backendProcess = spawn(exe, [], {
        cwd,
        stdio: 'pipe',
        env: { ...process.env, SERATO_NO_BROWSER: '1' }
    })

    backendProcess.stdout.on('data', d => process.stdout.write(`[backend] ${d}`))
    backendProcess.stderr.on('data', d => process.stderr.write(`[backend] ${d}`))
    backendProcess.on('exit', code => console.log(`Backend exited (code ${code})`))
}

// Fetch settings from the Go backend once its HTTP server is up.
async function fetchBackendSettings() {
    for (let i = 0; i < 20; i++) {
        try {
            const res = await fetch('http://localhost:8080/api/settings')
            if (res.ok) return await res.json()
        } catch (_) {}
        await new Promise(r => setTimeout(r, 500))
    }
    return null
}

function applySettings(st) {
    if (!st) return { ok: false }
    collapsedSize = {
        width: st.collapsed_width || collapsedSize.width,
        height: st.collapsed_height || collapsedSize.height
    }
    expandedSize = {
        width: st.expanded_width || expandedSize.width,
        height: st.expanded_height || expandedSize.height
    }
    const hotkeyOk = setHotkey(st.hotkey)
    setupTray(st)
    // Re-clamp current window into bounds with the new sizes
    if (mainWindow && !mainWindow.isDestroyed()) {
        const [x, y] = mainWindow.getPosition()
        const [w, h] = mainWindow.getSize()
        const target = (w > collapsedSize.width || h > collapsedSize.height) ? expandedSize : collapsedSize
        mainWindow.setBounds(clampRect(x, y, target.width, target.height))
    }
    return { ok: true, hotkeyOk }
}

// ── Window ─────────────────────────────────────────────────────────────────

function createWindow() {
    loadWindowState()

    const bounds = clampRect(
        winState.x !== null ? winState.x : 100,
        winState.y !== null ? winState.y : 100,
        collapsedSize.width,
        collapsedSize.height
    )

    mainWindow = new BrowserWindow({
        x: bounds.x,
        y: bounds.y,
        width: collapsedSize.width,
        height: collapsedSize.height,
        alwaysOnTop: true,
        frame: false,
        transparent: true,
        resizable: false,
        backgroundColor: '#00000000',
        title: 'Serato Companion',
        webPreferences: {
            preload: path.join(__dirname, 'preload.js'),
            contextIsolation: true,
            nodeIntegration: false
        }
    })

    // macOS: float above full-screen apps on all Mission Control spaces
    if (process.platform === 'darwin') {
        mainWindow.setAlwaysOnTop(true, 'floating')
        mainWindow.setVisibleOnAllWorkspaces(true, { visibleOnFullScreen: true })
    }

    mainWindow.on('move', scheduleSaveWindowState)
    mainWindow.on('resize', scheduleSaveWindowState)
    mainWindow.on('close', saveWindowState)
    mainWindow.on('closed', () => { mainWindow = null })

    // Give the Go HTTP server a moment to bind before loading
    const tryLoad = (attempts) => {
        mainWindow.loadURL('http://localhost:8080').catch(() => {
            if (attempts > 0) setTimeout(() => tryLoad(attempts - 1), 500)
        })
    }
    setTimeout(() => tryLoad(10), 800)

    // Pull user settings (window sizes, hotkey, tray) once the backend is up
    fetchBackendSettings().then(applySettings)
}

// ── Hotkey ─────────────────────────────────────────────────────────────────

let currentHotkey = ''

// setHotkey registers a global accelerator. Returns false when registration
// fails (invalid accelerator or taken by another app) so the UI can warn.
function setHotkey(accelerator) {
    if (accelerator === currentHotkey) return true
    if (currentHotkey) globalShortcut.unregister(currentHotkey)
    currentHotkey = accelerator || ''
    if (!currentHotkey) return true
    try {
        const ok = globalShortcut.register(currentHotkey, () => {
            if (mainWindow && !mainWindow.isDestroyed()) {
                mainWindow.webContents.send('toggle-expand')
            }
        })
        if (!ok) {
            console.error(`Hotkey registration failed: ${currentHotkey}`)
            currentHotkey = ''
            return false
        }
        return true
    } catch (err) {
        console.error(`Hotkey registration error: ${err.message}`)
        currentHotkey = ''
        return false
    }
}

// ── Tray (macOS) ───────────────────────────────────────────────────────────

function setupTray(st) {
    if (process.platform !== 'darwin' || !st || !st.tray_mode) return
    if (tray) return
    tray = new Tray(buildDragIcon())
    tray.setToolTip('Serato Companion')
    tray.on('click', () => {
        if (!mainWindow) return
        if (mainWindow.isVisible()) {
            mainWindow.hide()
        } else {
            mainWindow.show()
            mainWindow.moveTop()
        }
    })
}

// ── Drag icon ──────────────────────────────────────────────────────────────

function buildDragIcon() {
    const size = 32
    const buf  = Buffer.alloc(size * size * 4)
    for (let i = 0; i < size * size; i++) {
        const x = i % size
        const y = Math.floor(i / size)
        const inCircle = Math.hypot(x - 15.5, y - 15.5) < 13
        buf[i * 4 + 0] = inCircle ? 56  : 0
        buf[i * 4 + 1] = inCircle ? 189 : 0
        buf[i * 4 + 2] = inCircle ? 248 : 0
        buf[i * 4 + 3] = inCircle ? 220 : 0
    }
    return nativeImage.createFromBuffer(buf, { width: size, height: size })
}

const dragIcon = buildDragIcon()

// ── IPC ────────────────────────────────────────────────────────────────────

ipcMain.on('ondragstart', (event, filePath) => {
    event.sender.startDrag({ file: filePath, icon: dragIcon })
})

// Custom pill drag — transparent frameless windows can't rely on -webkit-app-region
ipcMain.on('move-window', (event, { dx, dy }) => {
    if (!mainWindow) return
    const [x, y] = mainWindow.getPosition()
    mainWindow.setPosition(x + dx, y + dy)
    scheduleSaveWindowState()
})

// Edge-aware expand: clamp the expanded rect into the current display's work
// area so expanding near a screen edge never clips the window. The pill stays
// anchored to its current position wherever that fits.
ipcMain.on('expand-window', () => {
    if (!mainWindow) return
    const [x, y] = mainWindow.getPosition()
    const b = clampRect(x, y, expandedSize.width, expandedSize.height)
    mainWindow.setResizable(true)
    mainWindow.setBounds(b, true)
    // stay resizable so user can drag window edges
})

ipcMain.on('collapse-window', () => {
    if (!mainWindow) return
    // Capture whatever size the user may have resized to before collapsing
    const [w, h] = mainWindow.getSize()
    if (w !== collapsedSize.width || h !== collapsedSize.height) {
        expandedSize = { width: w, height: h }
        winState.expandedW = w
        winState.expandedH = h
    }
    const [x, y] = mainWindow.getPosition()
    const b = clampRect(x, y, collapsedSize.width, collapsedSize.height)
    mainWindow.setResizable(true)
    mainWindow.setBounds(b, true)
    mainWindow.setResizable(false)
    scheduleSaveWindowState()
})

ipcMain.on('set-zoom', (event, factor) => {
    if (!mainWindow) return
    mainWindow.webContents.setZoomFactor(Math.max(0.4, Math.min(2.0, factor)))
})

ipcMain.handle('apply-settings', (event, st) => {
    return applySettings(st)
})

// ── App lifecycle ──────────────────────────────────────────────────────────

app.whenReady().then(() => {
    startBackend()
    createWindow()

    // On Windows all HWND_TOPMOST windows share one layer; whichever was
    // activated most recently sits on top.  Serato keeps bumping itself above
    // our window whenever it redraws.  moveTop() re-asserts our Z-position
    // without stealing keyboard focus.
    setInterval(() => {
        if (mainWindow && !mainWindow.isDestroyed()) mainWindow.moveTop()
    }, 1000)

    app.on('activate', () => {
        if (BrowserWindow.getAllWindows().length === 0) createWindow()
    })
})

app.on('will-quit', () => {
    globalShortcut.unregisterAll()
})

app.on('window-all-closed', () => {
    if (backendProcess) {
        backendProcess.kill()
        backendProcess = null
    }
    if (process.platform !== 'darwin') app.quit()
})
