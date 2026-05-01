const { contextBridge, ipcRenderer } = require('electron')

contextBridge.exposeInMainWorld('electronAPI', {
    startDrag:      (filePath)    => ipcRenderer.send('ondragstart', filePath),
    expandWindow:   ()            => ipcRenderer.send('expand-window'),
    collapseWindow: ()            => ipcRenderer.send('collapse-window'),
    moveWindowBy:   (dx, dy)      => ipcRenderer.send('move-window', { dx, dy }),
    setZoom:        (factor)      => ipcRenderer.send('set-zoom', factor),
    isElectron: true
})
