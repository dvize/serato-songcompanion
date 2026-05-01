const { app, BrowserWindow, ipcMain, nativeImage } = require('electron')
const path = require('path')
const { spawn } = require('child_process')

let mainWindow
let backendProcess

const COLLAPSED = { width: 340, height: 64 }
const EXPANDED  = { width: 860, height: 720 }

// Remembers the last user-chosen expanded size so resize is preserved across collapse/expand
let expandedSize = { ...EXPANDED }

// ── Paths ──────────────────────────────────────────────────────────────────

function backendBinaryName() {
    if (process.platform === 'win32') return 'companion.exe'
    if (process.arch === 'arm64')     return 'companion-mac-arm64'
    return 'companion-mac-amd64'
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
        try { require('fs').chmodSync(exe, 0o755) } catch (_) {}
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

// ── Window ─────────────────────────────────────────────────────────────────

function createWindow() {
    mainWindow = new BrowserWindow({
        width:  COLLAPSED.width,
        height: COLLAPSED.height,
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

    // Give the Go HTTP server a moment to bind before loading
    const tryLoad = (attempts) => {
        mainWindow.loadURL('http://localhost:8080').catch(() => {
            if (attempts > 0) setTimeout(() => tryLoad(attempts - 1), 500)
        })
    }
    setTimeout(() => tryLoad(10), 800)

    mainWindow.on('closed', () => { mainWindow = null })
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
})

ipcMain.on('expand-window', () => {
    if (!mainWindow) return
    mainWindow.setResizable(true)
    mainWindow.setSize(expandedSize.width, expandedSize.height, true)
    // stay resizable so user can drag window edges
})

ipcMain.on('collapse-window', () => {
    if (!mainWindow) return
    // Capture whatever size the user may have resized to before collapsing
    const [w, h] = mainWindow.getSize()
    if (w !== COLLAPSED.width || h !== COLLAPSED.height) {
        expandedSize = { width: w, height: h }
    }
    mainWindow.setResizable(true)
    mainWindow.setSize(COLLAPSED.width, COLLAPSED.height, true)
    mainWindow.setResizable(false)
})

ipcMain.on('set-zoom', (event, factor) => {
    if (!mainWindow) return
    mainWindow.webContents.setZoomFactor(Math.max(0.4, Math.min(2.0, factor)))
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

app.on('window-all-closed', () => {
    if (backendProcess) {
        backendProcess.kill()
        backendProcess = null
    }
    if (process.platform !== 'darwin') app.quit()
})
