package main

import (
	"crypto/sha256"
	"fmt"
	"fyne.io/fyne/v2/dialog"
)

// showFileHash computes SHA-256 hash of current editor content and shows it.
func (a *App) showFileHash() {
	if a.editor == nil {
		dialog.ShowInformation("File Hash", "No file open", a.mainWin)
		return
	}

	content := a.editor.GetFullText()
	hash := sha256.Sum256([]byte(content))
	dialog.ShowInformation("File Hash", fmt.Sprintf("SHA-256: %x", hash), a.mainWin)
}
