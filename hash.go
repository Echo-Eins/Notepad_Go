package main

import (
	"crypto/sha256"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// showFileHash computes SHA-256 hash of current editor content and shows it.
func (a *App) showFileHash() {
	if a.editor == nil {
		dialog.ShowInformation("File Hash", "No file open", a.mainWin)
		return
	}

	content := a.editor.GetFullText()
	hash := sha256.Sum256([]byte(content))
	hashStr := fmt.Sprintf("%x", hash)

	entry := widget.NewEntry()
	entry.SetText(hashStr)
	entry.Disable()

	copyBtn := widget.NewButton("Copy", func() {
		a.mainWin.Clipboard().SetContent(hashStr)
	})

	dialogContent := container.NewVBox(
		widget.NewLabel("SHA-256:"),
		entry,
		copyBtn,
	)

	hashDialog := dialog.NewCustom("File Hash", "Close", dialogContent, a.mainWin)
	hashDialog.Resize(fyne.NewSize(600, 150))
	hashDialog.Show()
	AnimateShow(dialogContent)
}
