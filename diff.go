package main

import (
	"fmt"
	"image/color"
	"os"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/sergi/go-diff/diffmatchpatch"
)

// compareFiles запускает выбор двух файлов и отображает различия между ними
func (a *App) compareFiles() {
	if a.dialogManager == nil {
		return
	}

	// Выбираем первый файл
	a.dialogManager.ShowOpenFileDialog(func(first string) {
		if first == "" {
			return
		}
		// Выбираем второй файл
		a.dialogManager.ShowOpenFileDialog(func(second string) {
			if second == "" {
				return
			}
			a.showDiffWindow(first, second)
		})
	})
}

// showDiffWindow отображает окно с подсветкой различий
func (a *App) showDiffWindow(file1, file2 string) {
	content1, err1 := os.ReadFile(file1)
	content2, err2 := os.ReadFile(file2)
	if err1 != nil || err2 != nil {
		dialog.ShowError(fmt.Errorf("failed to read files"), a.mainWin)
		return
	}

	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(string(content1), string(content2), false)

	grid := widget.NewTextGrid()
	row := 0
	for _, d := range diffs {
		lines := strings.Split(d.Text, "\n")
		for i, line := range lines {
			if i == len(lines)-1 && line == "" {
				continue
			}
			cells := make([]widget.TextGridCell, len([]rune(line)))
			for j, r := range line {
				cells[j] = widget.TextGridCell{Rune: r}
			}
			grid.SetRow(row, widget.TextGridRow{Cells: cells})
			style := &widget.CustomTextGridStyle{}
			switch d.Type {
			case diffmatchpatch.DiffInsert:
				style.BGColor = color.NRGBA{R: 0, G: 255, B: 0, A: 100}
			case diffmatchpatch.DiffDelete:
				style.BGColor = color.NRGBA{R: 255, G: 0, B: 0, A: 100}
			}
			if style.BGColor != nil {
				grid.SetRowStyle(row, style)
			}
			row++
		}
	}

	win := a.fyneApp.NewWindow("File Diff")
	win.SetContent(container.NewScroll(grid))
	win.Resize(fyne.NewSize(800, 600))
	win.Show()
}
