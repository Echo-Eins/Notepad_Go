package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// DialogManager управляет всеми диалогами приложения
type DialogManager struct {
	mainWindow fyne.Window
	editor     *EditorWidget
	config     *Config
}

// NewDialogManager создает новый менеджер диалогов
func NewDialogManager(window fyne.Window, editor *EditorWidget, config *Config) *DialogManager {
	return &DialogManager{
		mainWindow: window,
		editor:     editor,
		config:     config,
	}
}

// ShowOpenFileDialog показывает диалог открытия файла
func (dm *DialogManager) ShowOpenFileDialog(callback func(string)) {
	fileDialog := dialog.NewFileOpen(func(file fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, dm.mainWindow)
			return
		}
		if file == nil {
			return
		}

		path := file.URI().Path()
		file.Close()

		if callback != nil {
			callback(path)
		}
	}, dm.mainWindow)

	// Устанавливаем фильтры для поддерживаемых типов файлов
	fileDialog.SetFilter(storage.NewExtensionFileFilter([]string{
		".go", ".rs", ".py", ".c", ".h", ".java", ".js", ".ts",
		".txt", ".md", ".json", ".xml", ".yaml", ".yml",
	}))

	fileDialog.Show()
}

// ShowSaveFileDialog показывает диалог сохранения файла
func (dm *DialogManager) ShowSaveFileDialog(callback func(string)) {
	fileDialog := dialog.NewFileSave(func(file fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, dm.mainWindow)
			return
		}
		if file == nil {
			return
		}

		path := file.URI().Path()
		file.Close()

		if callback != nil {
			callback(path)
		}
	}, dm.mainWindow)

	// Устанавливаем имя файла по умолчанию
	if dm.editor != nil && dm.editor.fileName != "" {
		fileDialog.SetFileName(dm.editor.fileName)
	} else {
		fileDialog.SetFileName("untitled.txt")
	}

	fileDialog.Show()
}

// ShowFindDialog показывает диалог поиска
func (dm *DialogManager) ShowFindDialog(onFind func(string, bool, bool, bool)) {
	var searchEntry *widget.Entry
	var caseSensitiveCheck *widget.Check
	var wholeWordCheck *widget.Check
	var regexCheck *widget.Check

	searchEntry = widget.NewEntry()
	searchEntry.SetPlaceHolder("Enter text to find...")

	caseSensitiveCheck = widget.NewCheck("Case sensitive", nil)
	wholeWordCheck = widget.NewCheck("Whole words only", nil)
	regexCheck = widget.NewCheck("Use regular expressions", nil)

	// Загружаем настройки из конфигурации
	if dm.config != nil {
		caseSensitiveCheck.SetChecked(dm.config.Editor.SearchCaseSensitive)
		wholeWordCheck.SetChecked(dm.config.Editor.SearchWholeWord)
		regexCheck.SetChecked(dm.config.Editor.SearchRegex)
	}

	content := container.NewVBox(
		searchEntry,
		container.NewHBox(
			caseSensitiveCheck,
			wholeWordCheck,
			regexCheck,
		),
	)

	findDialog := dialog.NewCustomConfirm("Find", "Find", "Cancel", content, func(confirmed bool) {
		if confirmed && searchEntry.Text != "" {
			if onFind != nil {
				onFind(searchEntry.Text,
					caseSensitiveCheck.Checked,
					wholeWordCheck.Checked,
					regexCheck.Checked)
			}
		}
	}, dm.mainWindow)

	findDialog.Show()

	// Фокус на поле ввода
	dm.mainWindow.Canvas().Focus(searchEntry)
}

// ShowReplaceDialog показывает диалог замены
func (dm *DialogManager) ShowReplaceDialog(onReplace func(string, string, bool, bool, bool, bool)) {
	var findEntry, replaceEntry *widget.Entry
	var caseSensitiveCheck, wholeWordCheck, regexCheck, replaceAllCheck *widget.Check

	findEntry = widget.NewEntry()
	findEntry.SetPlaceHolder("Find what...")

	replaceEntry = widget.NewEntry()
	replaceEntry.SetPlaceHolder("Replace with...")

	caseSensitiveCheck = widget.NewCheck("Case sensitive", nil)
	wholeWordCheck = widget.NewCheck("Whole words only", nil)
	regexCheck = widget.NewCheck("Use regular expressions", nil)
	replaceAllCheck = widget.NewCheck("Replace all occurrences", nil)

	// Загружаем настройки
	if dm.config != nil {
		caseSensitiveCheck.SetChecked(dm.config.Editor.SearchCaseSensitive)
		wholeWordCheck.SetChecked(dm.config.Editor.SearchWholeWord)
		regexCheck.SetChecked(dm.config.Editor.SearchRegex)
	}

	content := container.NewVBox(
		widget.NewLabel("Find:"),
		findEntry,
		widget.NewLabel("Replace:"),
		replaceEntry,
		container.NewHBox(
			caseSensitiveCheck,
			wholeWordCheck,
		),
		container.NewHBox(
			regexCheck,
			replaceAllCheck,
		),
	)

	replaceDialog := dialog.NewCustomConfirm("Replace", "Replace", "Cancel", content, func(confirmed bool) {
		if confirmed && findEntry.Text != "" {
			if onReplace != nil {
				onReplace(findEntry.Text,
					replaceEntry.Text,
					caseSensitiveCheck.Checked,
					wholeWordCheck.Checked,
					regexCheck.Checked,
					replaceAllCheck.Checked)
			}
		}
	}, dm.mainWindow)

	replaceDialog.Show()
	dm.mainWindow.Canvas().Focus(findEntry)
}

// ShowGoToLineDialog показывает диалог перехода к строке
func (dm *DialogManager) ShowGoToLineDialog(maxLine int, onGoTo func(int)) {
	entry := widget.NewEntry()
	entry.SetPlaceHolder(fmt.Sprintf("Line number (1-%d)", maxLine))

	// Валидация - только числа
	entry.OnChanged = func(text string) {
		if text == "" {
			return
		}

		// Удаляем нецифровые символы
		cleaned := ""
		for _, r := range text {
			if r >= '0' && r <= '9' {
				cleaned += string(r)
			}
		}

		if cleaned != text {
			entry.SetText(cleaned)
		}
	}

	label := widget.NewLabel(fmt.Sprintf("Enter line number (1-%d):", maxLine))

	content := container.NewVBox(label, entry)

	goToDialog := dialog.NewCustomConfirm("Go to Line", "Go", "Cancel", content, func(confirmed bool) {
		if confirmed && entry.Text != "" {
			lineNum, err := strconv.Atoi(entry.Text)
			if err == nil && lineNum >= 1 && lineNum <= maxLine {
				if onGoTo != nil {
					onGoTo(lineNum)
				}
			} else {
				dialog.ShowError(fmt.Errorf("Invalid line number. Must be between 1 and %d", maxLine), dm.mainWindow)
			}
		}
	}, dm.mainWindow)

	goToDialog.Show()
	dm.mainWindow.Canvas().Focus(entry)
}

// ShowCommandPaletteDialog показывает командную палитру
func (dm *DialogManager) ShowCommandPaletteDialog(commands []Command, onExecute func(Command)) {
	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Type a command...")

	// Создаем список команд
	commandList := widget.NewList(
		func() int { return len(commands) },
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewIcon(theme.DocumentIcon()),
				widget.NewLabel("Command"),
				widget.NewLabel("Shortcut"),
			)
		},
		func(index widget.ListItemID, item fyne.CanvasObject) {
			container := item.(*container.HBox)
			icon := container.Objects[0].(*widget.Icon)
			label := container.Objects[1].(*widget.Label)
			shortcut := container.Objects[2].(*widget.Label)

			cmd := commands[index]
			icon.SetResource(cmd.Icon)
			label.SetText(cmd.Name)
			shortcut.SetText(cmd.Shortcut)
			shortcut.Importance = widget.LowImportance
		},
	)

	// Фильтрация команд при вводе
	filteredCommands := commands
	searchEntry.OnChanged = func(text string) {
		if text == "" {
			filteredCommands = commands
		} else {
			filteredCommands = filterCommands(commands, text)
		}
		commandList.Refresh()
	}

	// Выполнение команды при выборе
	commandList.OnSelected = func(index widget.ListItemID) {
		if index >= 0 && index < len(filteredCommands) {
			if onExecute != nil {
				onExecute(filteredCommands[index])
			}
		}
	}

	content := container.NewBorder(searchEntry, nil, nil, nil, commandList)

	paletteDialog := dialog.NewCustom("Command Palette", "Close", content, dm.mainWindow)
	paletteDialog.Resize(fyne.NewSize(600, 400))
	paletteDialog.Show()

	dm.mainWindow.Canvas().Focus(searchEntry)
}

// ShowPreferencesDialog показывает диалог настроек
func (dm *DialogManager) ShowPreferencesDialog(config *Config, onSave func(*Config)) {
	// Вкладки настроек
	tabs := container.NewAppTabs(
		container.NewTabItem("General", dm.createGeneralSettings(config)),
		container.NewTabItem("Editor", dm.createEditorSettings(config)),
		container.NewTabItem("Appearance", dm.createAppearanceSettings(config)),
		container.NewTabItem("Key Bindings", dm.createKeyBindingsSettings(config)),
		container.NewTabItem("Advanced", dm.createAdvancedSettings(config)),
	)

	prefsDialog := dialog.NewCustomConfirm("Preferences", "Save", "Cancel", tabs, func(confirmed bool) {
		if confirmed && onSave != nil {
			onSave(config)
		}
	}, dm.mainWindow)

	prefsDialog.Resize(fyne.NewSize(700, 500))
	prefsDialog.Show()
}

// createGeneralSettings создает вкладку общих настроек
func (dm *DialogManager) createGeneralSettings(config *Config) fyne.CanvasObject {
	// Язык интерфейса
	languageSelect := widget.NewSelect([]string{"English", "Russian"}, func(selected string) {
		if selected == "English" {
			config.App.Language = "en"
		} else {
			config.App.Language = "ru"
		}
	})
	if config.App.Language == "ru" {
		languageSelect.SetSelected("Russian")
	} else {
		languageSelect.SetSelected("English")
	}

	// Поведение при запуске
	startupSelect := widget.NewSelect([]string{"Blank", "Last Session", "Welcome Screen"}, func(selected string) {
		switch selected {
		case "Blank":
			config.App.StartupBehavior = "blank"
		case "Last Session":
			config.App.StartupBehavior = "last_session"
		case "Welcome Screen":
			config.App.StartupBehavior = "welcome"
		}
	})

	switch config.App.StartupBehavior {
	case "blank":
		startupSelect.SetSelected("Blank")
	case "last_session":
		startupSelect.SetSelected("Last Session")
	case "welcome":
		startupSelect.SetSelected("Welcome Screen")
	}

	// Проверка обновлений
	checkUpdatesCheck := widget.NewCheck("Check for updates on startup", func(checked bool) {
		config.App.CheckUpdates = checked
	})
	checkUpdatesCheck.SetChecked(config.App.CheckUpdates)

	// Автосохранение
	autoSaveCheck := widget.NewCheck("Enable auto-save", func(checked bool) {
		config.Editor.AutoSave = checked
	})
	autoSaveCheck.SetChecked(config.Editor.AutoSave)

	autoSaveDelayEntry := widget.NewEntry()
	autoSaveDelayEntry.SetText(strconv.Itoa(config.Editor.AutoSaveDelay))
	autoSaveDelayEntry.OnChanged = func(text string) {
		if delay, err := strconv.Atoi(text); err == nil && delay > 0 {
			config.Editor.AutoSaveDelay = delay
		}
	}

	return container.NewVBox(
		widget.NewCard("Interface", "", container.NewVBox(
			container.NewHBox(widget.NewLabel("Language:"), languageSelect),
			container.NewHBox(widget.NewLabel("On startup:"), startupSelect),
			checkUpdatesCheck,
		)),
		widget.NewCard("Files", "", container.NewVBox(
			autoSaveCheck,
			container.NewHBox(
				widget.NewLabel("Auto-save delay (seconds):"),
				autoSaveDelayEntry,
			),
		)),
		widget.NewSeparator(),
	)
}

// createEditorSettings создает вкладку настроек редактора
func (dm *DialogManager) createEditorSettings(config *Config) fyne.CanvasObject {
	// Размер табуляции
	tabSizeEntry := widget.NewEntry()
	tabSizeEntry.SetText(strconv.Itoa(config.Editor.TabSize))
	tabSizeEntry.OnChanged = func(text string) {
		if size, err := strconv.Atoi(text); err == nil && size > 0 && size <= 16 {
			config.Editor.TabSize = size
		}
	}

	// Использовать пробелы вместо табуляции
	useSpacesCheck := widget.NewCheck("Insert spaces instead of tabs", func(checked bool) {
		config.Editor.UseSpaces = checked
	})
	useSpacesCheck.SetChecked(config.Editor.UseSpaces)

	// Показывать номера строк
	showLineNumbersCheck := widget.NewCheck("Show line numbers", func(checked bool) {
		config.Editor.ShowLineNumbers = checked
	})
	showLineNumbersCheck.SetChecked(config.Editor.ShowLineNumbers)

	// Перенос слов
	wordWrapCheck := widget.NewCheck("Word wrap", func(checked bool) {
		config.Editor.WordWrap = checked
	})
	wordWrapCheck.SetChecked(config.Editor.WordWrap)

	// Направляющие отступов
	indentGuidesCheck := widget.NewCheck("Show indent guides", func(checked bool) {
		config.Editor.IndentGuides = checked
	})
	indentGuidesCheck.SetChecked(config.Editor.IndentGuides)

	// Подсветка текущей строки
	highlightCurrentLineCheck := widget.NewCheck("Highlight current line", func(checked bool) {
		config.Editor.HighlightCurrentLine = checked
	})
	highlightCurrentLineCheck.SetChecked(config.Editor.HighlightCurrentLine)

	// Vim режим
	vimModeCheck := widget.NewCheck("Enable Vim mode", func(checked bool) {
		config.Editor.VimMode = checked
	})
	vimModeCheck.SetChecked(config.Editor.VimMode)

	return container.NewVBox(
		widget.NewCard("Indentation", "", container.NewVBox(
			container.NewHBox(widget.NewLabel("Tab size:"), tabSizeEntry),
			useSpacesCheck,
			indentGuidesCheck,
		)),
		widget.NewCard("Display", "", container.NewVBox(
			showLineNumbersCheck,
			wordWrapCheck,
			highlightCurrentLineCheck,
		)),
		widget.NewCard("Input modes", "", container.NewVBox(
			vimModeCheck,
		)),
	)
}

// createAppearanceSettings создает вкладку настроек внешнего вида
func (dm *DialogManager) createAppearanceSettings(config *Config) fyne.CanvasObject {
	// Тема
	themeSelect := widget.NewSelect([]string{"Dark", "Light", "Auto"}, func(selected string) {
		config.App.Theme = strings.ToLower(selected)
	})
	themeSelect.SetSelected(strings.Title(config.App.Theme))

	// Размер шрифта
	fontSizeSlider := widget.NewSlider(10, 24)
	fontSizeSlider.Value = float64(config.Editor.FontSize)
	fontSizeSlider.Step = 1
	fontSizeLabel := widget.NewLabel(fmt.Sprintf("%.0f", fontSizeSlider.Value))

	fontSizeSlider.OnChanged = func(value float64) {
		config.Editor.FontSize = float32(value)
		fontSizeLabel.SetText(fmt.Sprintf("%.0f", value))
	}

	// Показывать миниатюру
	showMinimapCheck := widget.NewCheck("Show minimap", func(checked bool) {
		config.Minimap.IsVisible = checked
	})
	showMinimapCheck.SetChecked(config.Minimap.IsVisible)

	// Показывать боковую панель
	showSidebarCheck := widget.NewCheck("Show sidebar", func(checked bool) {
		config.Sidebar.IsVisible = checked
	})
	showSidebarCheck.SetChecked(config.Sidebar.IsVisible)

	return container.NewVBox(
		widget.NewCard("Theme", "", container.NewVBox(
			container.NewHBox(widget.NewLabel("Color theme:"), themeSelect),
		)),
		widget.NewCard("Font", "", container.NewVBox(
			container.NewHBox(
				widget.NewLabel("Font size:"),
				fontSizeSlider,
				fontSizeLabel,
			),
		)),
		widget.NewCard("Layout", "", container.NewVBox(
			showMinimapCheck,
			showSidebarCheck,
		)),
	)
}

// createKeyBindingsSettings создает вкладку настроек горячих клавиш
func (dm *DialogManager) createKeyBindingsSettings(config *Config) fyne.CanvasObject {
	// Создаем список горячих клавиш
	keyBindings := []struct {
		Name    string
		Current string
		Action  string
	}{
		{"New File", config.KeyBindings.NewFile, "new_file"},
		{"Open File", config.KeyBindings.OpenFile, "open_file"},
		{"Save File", config.KeyBindings.SaveFile, "save_file"},
		{"Find", config.KeyBindings.Find, "find"},
		{"Replace", config.KeyBindings.Replace, "replace"},
		{"Go to Line", config.KeyBindings.GoToLine, "go_to_line"},
		{"Toggle Sidebar", config.KeyBindings.ToggleSidebar, "toggle_sidebar"},
		{"Toggle Minimap", config.KeyBindings.ToggleMinimap, "toggle_minimap"},
		{"Command Palette", config.KeyBindings.CommandPalette, "command_palette"},
	}

	list := widget.NewList(
		func() int { return len(keyBindings) },
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewLabel("Action"),
				widget.NewButton("Change", nil),
			)
		},
		func(index widget.ListItemID, item fyne.CanvasObject) {
			container := item.(*container.HBox)
			label := container.Objects[0].(*widget.Label)
			button := container.Objects[1].(*widget.Button)

			kb := keyBindings[index]
			label.SetText(fmt.Sprintf("%s: %s", kb.Name, kb.Current))
			button.OnTapped = func() {
				// TODO: Implement key binding change dialog
			}
		},
	)

	// Режимы клавиш
	vimBindingsCheck := widget.NewCheck("Enable Vim key bindings", func(checked bool) {
		config.KeyBindings.EnableVimBindings = checked
	})
	vimBindingsCheck.SetChecked(config.KeyBindings.EnableVimBindings)

	emacsBindingsCheck := widget.NewCheck("Enable Emacs key bindings", func(checked bool) {
		config.KeyBindings.EnableEmacsBindings = checked
	})
	emacsBindingsCheck.SetChecked(config.KeyBindings.EnableEmacsBindings)

	vscodeBindingsCheck := widget.NewCheck("Use VS Code compatible bindings", func(checked bool) {
		config.KeyBindings.EnableVSCodeBindings = checked
	})
	vscodeBindingsCheck.SetChecked(config.KeyBindings.EnableVSCodeBindings)

	return container.NewVBox(
		widget.NewCard("Key Bindings", "", list),
		widget.NewCard("Key Binding Modes", "", container.NewVBox(
			vimBindingsCheck,
			emacsBindingsCheck,
			vscodeBindingsCheck,
		)),
	)
}

// createAdvancedSettings создает вкладку расширенных настроек
func (dm *DialogManager) createAdvancedSettings(config *Config) fyne.CanvasObject {
	// Логирование
	enableLoggingCheck := widget.NewCheck("Enable logging", func(checked bool) {
		config.Advanced.EnableLogging = checked
	})
	enableLoggingCheck.SetChecked(config.Advanced.EnableLogging)

	logLevelSelect := widget.NewSelect([]string{"Debug", "Info", "Warning", "Error"}, func(selected string) {
		config.Advanced.LogLevel = strings.ToLower(selected)
	})
	logLevelSelect.SetSelected(strings.Title(config.Advanced.LogLevel))

	// Кэширование
	enableCachingCheck := widget.NewCheck("Enable caching", func(checked bool) {
		config.Advanced.EnableCaching = checked
	})
	enableCachingCheck.SetChecked(config.Advanced.EnableCaching)

	// Экспериментальные функции
	experimentalCard := widget.NewCard("Experimental Features", "⚠️ Use at your own risk", container.NewVBox())
	for feature, enabled := range config.Advanced.ExperimentalFeatures {
		featureName := feature // Capture for closure
		check := widget.NewCheck(strings.ReplaceAll(featureName, "_", " "), func(checked bool) {
			config.Advanced.ExperimentalFeatures[featureName] = checked
		})
		check.SetChecked(enabled)
		experimentalCard.Content.(*container.VBox).Add(check)
	}

	return container.NewVBox(
		widget.NewCard("Logging", "", container.NewVBox(
			enableLoggingCheck,
			container.NewHBox(widget.NewLabel("Log level:"), logLevelSelect),
		)),
		widget.NewCard("Performance", "", container.NewVBox(
			enableCachingCheck,
		)),
		experimentalCard,
	)
}

// ShowAboutDialog показывает диалог "О программе"
func (dm *DialogManager) ShowAboutDialog() {
	logo := widget.NewIcon(theme.DocumentIcon())

	title := widget.NewLabelWithStyle("Programmer's Notepad",
		fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	version := widget.NewLabel("Version 1.0.0")
	version.Alignment = fyne.TextAlignCenter

	description := widget.NewLabel("A modern code editor for programmers\nBuilt with Go and Fyne")
	description.Alignment = fyne.TextAlignCenter
	description.Wrapping = fyne.TextWrapWord

	copyright := widget.NewLabel("© 2024 Programmer's Notepad")
	copyright.Alignment = fyne.TextAlignCenter
	copyright.Importance = widget.LowImportance

	content := container.NewVBox(
		container.NewCenter(logo),
		title,
		version,
		widget.NewSeparator(),
		description,
		widget.NewSeparator(),
		copyright,
	)

	aboutDialog := dialog.NewCustom("About Programmer's Notepad", "OK", content, dm.mainWindow)
	aboutDialog.Show()
}

// filterCommands фильтрует команды по тексту поиска (fuzzy search)
func filterCommands(commands []Command, searchText string) []Command {
	searchText = strings.ToLower(searchText)
	var filtered []Command

	for _, cmd := range commands {
		name := strings.ToLower(cmd.Name)
		desc := strings.ToLower(cmd.Description)

		// Проверяем вхождение в название или описание
		if strings.Contains(name, searchText) || strings.Contains(desc, searchText) {
			filtered = append(filtered, cmd)
			continue
		}

		// Fuzzy matching для названия
		if fuzzyMatch(name, searchText) {
			filtered = append(filtered, cmd)
		}
	}

	return filtered
}

// fuzzyMatch выполняет нечеткое сопоставление строк
func fuzzyMatch(text, pattern string) bool {
	textRunes := []rune(text)
	patternRunes := []rune(pattern)

	textIndex := 0
	patternIndex := 0

	for textIndex < len(textRunes) && patternIndex < len(patternRunes) {
		if textRunes[textIndex] == patternRunes[patternIndex] {
			patternIndex++
		}
		textIndex++
	}

	return patternIndex == len(patternRunes)
}

// ShowErrorDialog показывает диалог ошибки
func (dm *DialogManager) ShowErrorDialog(err error) {
	dialog.ShowError(err, dm.mainWindow)
}

// ShowConfirmDialog показывает диалог подтверждения
func (dm *DialogManager) ShowConfirmDialog(title, message string, onConfirm func()) {
	dialog.ShowConfirm(title, message, func(confirmed bool) {
		if confirmed && onConfirm != nil {
			onConfirm()
		}
	}, dm.mainWindow)
}

// ShowInformationDialog показывает информационный диалог
func (dm *DialogManager) ShowInformationDialog(title, message string) {
	dialog.ShowInformation(title, message, dm.mainWindow)
}
