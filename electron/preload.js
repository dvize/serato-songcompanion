const { contextBridge, ipcRenderer } = require('electron')

contextBridge.exposeInMainWorld('electronAPI', {
    startDrag:       (filePath)  => ipcRenderer.send('ondragstart', filePath),
    expandWindow:    ()          => ipcRenderer.send('expand-window'),
    collapseWindow:  ()          => ipcRenderer.send('collapse-window'),
    moveWindowBy:    (dx, dy)    => ipcRenderer.send('move-window', { dx, dy }),
    setZoom:         (factor)    => ipcRenderer.send('set-zoom', factor),
    settingsChanged: (settings)  => ipcRenderer.invoke('apply-settings', settings),
    resetWindowSize: ()          => ipcRenderer.invoke('reset-window-size'),
    showContextMenu: ()          => ipcRenderer.send('window-context-menu'),
    onToggleExpand:  (cb)        => ipcRenderer.on('toggle-expand', () => cb()),
    isElectron: true
})
