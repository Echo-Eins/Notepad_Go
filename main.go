package main

import (
	"errors"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"os"
	"path/filepath"
	"strings"
)

type Command struct {
	Name     string
	Shortcut string
	Icon     fyne.Resource
	Action   func()
	Category string
}

// App представляет основное приложение
type App struct {
	fyneApp            fyne.App
	mainWin            fyne.Window
	editor             *EditorWidget
	sidebar            *SidebarWidget
	minimap            *MinimapWidget
	config             *Config
	configManager      *ConfigManager
	hotkeyManager      *HotkeyManager
	dialogManager      *DialogManager
	terminalMgr        *TerminalManager
	mainContent        fyne.CanvasObject
	currentFile        string
	recentFiles        []string
	commandHistory     *CommandHistory
	findDialog         dialog.Dialog
	searchResults      []TextRange
	currentSearchIndex int
	lastSearchText     string
	statusBar          *widget.Label
	appTheme           *AppTheme
}

// NewApp создает новое приложение
func NewApp() *App {
	myApp := app.NewWithID("dev.notepad.programmer")
	myApp.SetIcon(theme.DocumentIcon())

	// Загружаем конфигурацию
	configMgr := NewConfigManager("")
	config, err := configMgr.LoadConfig()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		config = DefaultConfig()
	}

	// Устанавливаем тему с учетом размера шрифта и радиуса скругления
	var baseTheme fyne.Theme
	switch config.App.Theme {

	case "light":
		baseTheme = NewLightTheme(config.App.CornerRadius)
	default:
		baseTheme = NewDarkTheme(config.App.CornerRadius)
	}
	appTheme := NewAppTheme(baseTheme, config.Editor.FontSize, config.App.CornerRadius)
	myApp.Settings().SetTheme(appTheme)

	mainWin := myApp.NewWindow("Programmer's Notepad")

	// Восстанавливаем размер и позицию окна
	mainWin.Resize(fyne.NewSize(float32(config.App.WindowWidth), float32(config.App.WindowHeight)))
	if config.App.WindowMaximized {
		mainWin.SetFullScreen(true)
	}

	appInstance := &App{
		fyneApp:        myApp,
		mainWin:        mainWin,
		config:         config,
		configManager:  configMgr,
		recentFiles:    config.App.LastOpenedFiles,
		commandHistory: NewCommandHistory(100),
		appTheme:       appTheme,
	}

	return appInstance
}

// setupUI создает и настраивает пользовательский интерфейс
func (a *App) setupUI() {
	// Создаем основные компоненты
	a.editor = NewEditor(a.config)
	a.sidebar = NewSidebar(a.config)
	a.minimap = NewMinimap(a.editor)

	// Создаем менеджеры
	a.dialogManager = NewDialogManager(a.mainWin, a.editor, a.config)
	a.terminalMgr = NewTerminalManager(a.config)
	a.hotkeyManager = NewHotkeyManager(a.config, a.mainWin)

	// Передаем ссылку на App в HotkeyManager для доступа к методам
	a.hotkeyManager.SetApp(a)

	// Настраиваем callbacks
	a.setupCallbacks()

	// Создаем меню
	a.createMainMenu()

	// Создаем основной layout
	a.createMainLayout()

	// Устанавливаем горячие клавиши
	a.setupHotkeys()

	// Загружаем последнюю сессию если настроено
	if a.config.App.StartupBehavior == "last_session" && len(a.recentFiles) > 0 {
		a.loadFile(a.recentFiles[0])
	}

	// Устанавливаем начальную директорию для сайдбара
	homeDir, _ := os.UserHomeDir()
	a.sidebar.SetRootPath(homeDir)
}

// createMainMenu создает главное меню
func (a *App) createMainMenu() {
	fileMenu := fyne.NewMenu("File",
		fyne.NewMenuItem("New", a.newFile),
		fyne.NewMenuItem("Open...", func() { a.openFile() }),
		fyne.NewMenuItem("Save", a.saveFile),
		fyne.NewMenuItem("Save As...", a.saveAsFile),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Recent Files", a.showRecentFiles),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Exit", func() {
			a.checkAndExit()
		}),
	)

	editMenu := fyne.NewMenu("Edit",
		fyne.NewMenuItem("Undo", a.undo),
		fyne.NewMenuItem("Redo", a.redo),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Cut", a.cut),
		fyne.NewMenuItem("Copy", a.copy),
		fyne.NewMenuItem("Paste", a.paste),
		fyne.NewMenuItem("Select All", a.selectAll),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Find...", a.showFind),
		fyne.NewMenuItem("Replace...", a.showReplace),
		fyne.NewMenuItem("Find in Files...", a.showFindInFiles),
		fyne.NewMenuItem("Go to Line...", a.showGoToLine),
		fyne.NewMenuItem("Go to Symbol...", a.showGoToSymbol),
	)

	viewMenu := fyne.NewMenu("View",
		fyne.NewMenuItem("Toggle Sidebar", a.toggleSidebar),
		fyne.NewMenuItem("Toggle Minimap", a.toggleMinimap),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Command Palette", a.showCommandPalette),
		fyne.NewMenuItem("File Explorer", a.focusFileExplorer),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Zoom In", a.zoomIn),
		fyne.NewMenuItem("Zoom Out", a.zoomOut),
		fyne.NewMenuItem("Reset Zoom", a.resetZoom),
	)

	toolsMenu := fyne.NewMenu("Tools",
		fyne.NewMenuItem("Terminal (CMD)", a.openCMD),
		fyne.NewMenuItem("Terminal (PowerShell)", a.openPowerShell),
		fyne.NewMenuItem("Custom Tools", a.showCustomTools),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Format Code", a.formatCode),
		fyne.NewMenuItem("Lint Code", a.lintCode),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Run File", a.runFile),
		fyne.NewMenuItem("Build Project", a.buildProject),
	)

	settingsMenu := fyne.NewMenu("Settings",
		fyne.NewMenuItem("Preferences", a.showPreferences),
		fyne.NewMenuItem("Key Bindings", a.showKeyBindings),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("About", a.showAbout),
	)

	mainMenu := fyne.NewMainMenu(fileMenu, editMenu, viewMenu, toolsMenu, settingsMenu)
	a.mainWin.SetMainMenu(mainMenu)
}

// createMainLayout создает основной layout
func (a *App) createMainLayout() {
	// Создаем статус бар
	statusBarContainer := a.createStatusBar()

	// Основной контент с редактором и миниатюрой
	var editorContent fyne.CanvasObject
	if a.config.Minimap.IsVisible {
		editorContent = container.NewBorder(nil, nil, nil, a.minimap, a.editor)
	} else {
		editorContent = a.editor
	}

	// Добавляем боковую панель если видима
	if a.config.Sidebar.IsVisible {
		a.mainContent = container.NewBorder(nil, statusBarContainer, a.sidebar, nil, editorContent)
	} else {
		a.mainContent = container.NewBorder(nil, statusBarContainer, nil, nil, editorContent)
	}

	a.mainWin.SetContent(a.mainContent)
}

// createStatusBar создает статусную строку
func (a *App) createStatusBar() fyne.CanvasObject {
	// Информация о файле
	fileLabel := widget.NewLabel("Ready")
	fileLabel.TextStyle.Italic = true

	// Позиция курсора
	positionLabel := widget.NewLabel("Ln 1, Col 1")
	a.statusBar = positionLabel

	// Язык/режим
	modeLabel := widget.NewLabel("Plain Text")

	// Кодировка
	encodingLabel := widget.NewLabel("UTF-8")

	// Разделители
	sep1 := widget.NewSeparator()
	sep2 := widget.NewSeparator()
	sep3 := widget.NewSeparator()

	statusContainer := container.NewHBox(
		fileLabel,
		sep1,
		positionLabel,
		sep2,
		modeLabel,
		sep3,
		encodingLabel,
	)

	return statusContainer
}

// setupCallbacks настраивает обратные вызовы между компонентами
func (a *App) setupCallbacks() {
	// Callbacks для редактора
	if a.editor != nil {
		a.editor.onContentChanged = func(content string) {
			// Обновляем миниатюру
			if a.minimap != nil {
				a.minimap.SetContent(content)
			}
		}

		a.editor.onCursorChanged = func(row, col int) {
			// Обновляем статус бар
			a.updateStatusBar(row, col)
		}

		a.editor.onFileChanged = func(filepath string) {
			a.currentFile = filepath
			a.updateTitle()
			a.addToRecentFiles(filepath)
		}
	}

	// Callbacks для сайдбара
	if a.sidebar != nil {
		a.sidebar.SetCallbacks(
			func(path string) { // onFileSelected
				// Предпросмотр файла
			},
			func(path string) { // onFileOpened
				a.loadFile(path)
			},
			func(path string) { // onPathChanged
				// Обновляем рабочую директорию
				if a.terminalMgr != nil {
					a.terminalMgr.SetWorkingDirectory(path)
				}
			},
		)
	}

	// Callbacks для миниатюры
	if a.minimap != nil {
		a.minimap.SetCallbacks(
			func(position float32) { // onScrollTo
				// Прокручиваем редактор
				a.scrollEditorTo(position)
			},
			func(line int) { // onLineClick
				// Переходим к строке
				a.goToLine(line + 1)
			},
		)
	}
}

// setupHotkeys настраивает горячие клавиши
func (a *App) setupHotkeys() {
	if a.hotkeyManager == nil {
		return
	}

	// Регистрируем действия приложения
	a.hotkeyManager.RegisterCustomAction("app_new_file", func(ctx HotkeyContext) bool {
		a.newFile()
		return true
	})

	a.hotkeyManager.RegisterCustomAction("app_open_file", func(ctx HotkeyContext) bool {
		a.openFile()
		return true
	})

	a.hotkeyManager.RegisterCustomAction("app_save_file", func(ctx HotkeyContext) bool {
		a.saveFile()
		return true
	})

	a.hotkeyManager.RegisterCustomAction("app_find", func(ctx HotkeyContext) bool {
		a.showFind()
		return true
	})

	a.hotkeyManager.RegisterCustomAction("app_toggle_sidebar", func(ctx HotkeyContext) bool {
		a.toggleSidebar()
		return true
	})

	a.hotkeyManager.RegisterCustomAction("app_toggle_minimap", func(ctx HotkeyContext) bool {
		a.toggleMinimap()
		return true
	})
}

// File operations

func (a *App) newFile() {
	// Проверяем несохраненные изменения
	if a.editor != nil && a.editor.IsDirty() {
		dialog.ShowConfirm("Unsaved Changes",
			"Do you want to save the current file?",
			func(save bool) {
				if save {
					a.saveFile()
				}
				a.createNewFile()
			}, a.mainWin)
	} else {
		a.createNewFile()
	}
}

func (a *App) createNewFile() {
	if a.editor != nil {
		a.editor.SetContent("")
		a.editor.filePath = ""
		a.editor.fileName = "untitled.txt"
		a.editor.isDirty = false
		a.currentFile = ""
		a.updateTitle()
	}
}

func (a *App) openFile() {
	a.dialogManager.ShowOpenFileDialog(func(path string) {
		a.loadFile(path)
	})
}

func (a *App) loadFile(path string) {
	if a.editor != nil {
		err := a.editor.LoadFile(path)
		if err != nil {
			if errors.Is(err, ErrFileTooLarge) {
				return
			}
			dialog.ShowError(err, a.mainWin)
		} else {
			a.currentFile = path
			a.updateTitle()
			a.addToRecentFiles(path)
		}
	}
}

func (a *App) saveFile() {
	if a.editor == nil {
		return
	}

	if a.currentFile == "" || a.editor.filePath == "" {
		a.saveAsFile()
		return
	}

	err := a.editor.SaveFile()
	if err != nil {
		dialog.ShowError(err, a.mainWin)
	} else {
		a.updateTitle()
	}
}

func (a *App) saveAsFile() {
	if a.editor == nil {
		return
	}

	a.dialogManager.ShowSaveFileDialog(func(path string) {
		err := a.editor.SaveAsFile(path)
		if err != nil {
			dialog.ShowError(err, a.mainWin)
		} else {
			a.currentFile = path
			a.updateTitle()
			a.addToRecentFiles(path)
		}
	})
}

// Edit operations - Полная реализация

func (a *App) undo() {
	if a.editor == nil || a.commandHistory == nil {
		return
	}

	err := a.commandHistory.Undo(a.editor)
	if err != nil {
		// Нет действий для отмены
		return
	}

	a.editor.updateDisplay()
}

func (a *App) redo() {
	if a.editor == nil || a.commandHistory == nil {
		return
	}

	err := a.commandHistory.Redo(a.editor)
	if err != nil {
		// Нет действий для повтора
		return
	}

	a.editor.updateDisplay()
}

func (a *App) cut() {
	if a.editor == nil {
		return
	}

	// Получаем выделенный текст
	selectedText := a.getSelectedText()
	if selectedText == "" {
		return
	}

	// Копируем в буфер обмена
	clipboard := a.mainWin.Clipboard()
	clipboard.SetContent(selectedText)

	// Удаляем выделенный текст
	a.deleteSelectedText()
}

func (a *App) copy() {
	if a.editor == nil {
		return
	}

	// Получаем выделенный текст
	selectedText := a.getSelectedText()
	if selectedText == "" {
		return
	}

	// Копируем в буфер обмена
	clipboard := a.mainWin.Clipboard()
	clipboard.SetContent(selectedText)
}

func (a *App) paste() {
	if a.editor == nil {
		return
	}

	// Получаем текст из буфера обмена
	clipboard := a.mainWin.Clipboard()
	content := clipboard.Content()

	if content == "" {
		return
	}

	// Вставляем текст в позицию курсора
	cmd := &InsertTextCommand{
		position: TextPosition{
			Row: a.editor.cursorRow,
			Col: a.editor.cursorCol,
		},
		text: content,
	}

	a.commandHistory.Execute(cmd, a.editor)
}

func (a *App) selectAll() {
	if a.editor == nil {
		return
	}

	cmd := &SelectAllCommand{}
	a.commandHistory.Execute(cmd, a.editor)
}

// Search operations - Полная реализация

func (a *App) showFind() {
	if a.editor == nil {
		return
	}

	a.dialogManager.ShowFindDialog(func(text string, caseSensitive, wholeWord, regex bool) {
		a.lastSearchText = text
		// Выполняем поиск
		cmd := &FindCommand{
			searchTerm:    text,
			caseSensitive: caseSensitive,
			wholeWord:     wholeWord,
			useRegex:      regex,
		}

		err := cmd.Execute(a.editor)
		if err != nil {
			dialog.ShowError(err, a.mainWin)
			return
		}

		// Сохраняем результаты поиска
		a.searchResults = a.editor.searchResults
		a.currentSearchIndex = 0

		// Переходим к первому результату
		if len(a.searchResults) > 0 {
			a.goToSearchResult(0)
		} else {
			dialog.ShowInformation("Find", "No matches found", a.mainWin)
		}
	})
}

func (a *App) showReplace() {
	if a.editor == nil {
		return
	}

	a.dialogManager.ShowReplaceDialog(func(find, replace string, caseSensitive, wholeWord, regex, replaceAll bool) {
		if replaceAll {
			// Заменяем все вхождения
			cmd := &ReplaceTextCommand{
				findText:    find,
				replaceText: replace,
			}
			a.commandHistory.Execute(cmd, a.editor)
		} else {
			// Заменяем текущее вхождение
			a.replaceCurrentMatch(find, replace)
		}
	})
}

func (a *App) showFindInFiles() {
	// Создаем диалог поиска в файлах
	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Search text...")

	pathEntry := widget.NewEntry()
	if a.sidebar != nil {
		pathEntry.SetText(a.sidebar.GetCurrentPath())
	}

	includeEntry := widget.NewEntry()
	includeEntry.SetPlaceHolder("*.go,*.rs,*.py")

	excludeEntry := widget.NewEntry()
	excludeEntry.SetPlaceHolder("*.log,*.tmp")

	content := container.NewVBox(
		widget.NewLabel("Search for:"),
		searchEntry,
		widget.NewLabel("In folder:"),
		pathEntry,
		widget.NewLabel("Include files:"),
		includeEntry,
		widget.NewLabel("Exclude files:"),
		excludeEntry,
	)

	dialog.ShowCustomConfirm("Find in Files", "Search", "Cancel", content, func(confirmed bool) {
		if confirmed && searchEntry.Text != "" {
			// Выполняем поиск в файлах
			a.performFindInFiles(searchEntry.Text, pathEntry.Text, includeEntry.Text, excludeEntry.Text)
		}
	}, a.mainWin)
}

func (a *App) showGoToLine() {
	if a.editor == nil {
		return
	}

	maxLine := a.editor.getLineCount()
	a.dialogManager.ShowGoToLineDialog(maxLine, func(line int) {
		a.goToLine(line)
	})
}

func (a *App) goToLine(line int) {
	if a.editor == nil {
		return
	}

	cmd := &GoToLineCommand{
		lineNumber: line,
	}

	err := cmd.Execute(a.editor)
	if err != nil {
		dialog.ShowError(err, a.mainWin)
	}
}

func (a *App) showGoToSymbol() {
	if a.editor == nil || a.currentFile == "" {
		return
	}

	// Анализируем код для поиска символов
	analyzer := NewCodeAnalyzer()

	// Определяем язык по расширению файла
	ext := filepath.Ext(a.currentFile)
	language := getLanguageByExtension(ext)

	// Находим функции и классы
	functions := analyzer.FindFunctions(a.editor.textContent, language)
	classes := analyzer.FindClasses(a.editor.textContent, language)

	// Объединяем все символы
	var symbols []CodeElement
	symbols = append(symbols, functions...)
	symbols = append(symbols, classes...)

	if len(symbols) == 0 {
		dialog.ShowInformation("Go to Symbol", "No symbols found", a.mainWin)
		return
	}

	// Создаем список символов
	symbolNames := make([]string, len(symbols))
	for i, sym := range symbols {
		symbolNames[i] = fmt.Sprintf("%s: %s", sym.Type, sym.Name)
	}

	// Показываем диалог выбора
	symbolList := widget.NewList(
		func() int { return len(symbolNames) },
		func() fyne.CanvasObject {
			return widget.NewLabel("Symbol")
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			o.(*widget.Label).SetText(symbolNames[i])
		},
	)

	symbolList.OnSelected = func(id widget.ListItemID) {
		// Переходим к выбранному символу
		if id < len(symbols) {
			a.goToLine(symbols[id].Line)
		}
	}

	symbolDialog := dialog.NewCustom("Go to Symbol", "Close",
		container.NewScroll(symbolList), a.mainWin)
	symbolDialog.Resize(fyne.NewSize(400, 500))
	symbolDialog.Show()
}

// View operations

func (a *App) toggleSidebar() {
	if a.sidebar == nil {
		return
	}

	isVisible := a.sidebar.IsVisible()
	a.sidebar.SetVisible(!isVisible)
	a.config.Sidebar.IsVisible = !isVisible

	// Пересоздаем layout
	a.createMainLayout()

	// Сохраняем настройки
	a.configManager.SaveConfig()
}

func (a *App) toggleMinimap() {
	if a.minimap == nil {
		return
	}

	isVisible := a.minimap.IsVisible()
	a.minimap.SetVisible(!isVisible)
	a.config.Minimap.IsVisible = !isVisible

	// Пересоздаем layout
	a.createMainLayout()

	// Сохраняем настройки
	a.configManager.SaveConfig()
}

func (a *App) showCommandPalette() {
	commands := a.getAvailableCommands()
	a.dialogManager.ShowCommandPaletteDialog(commands, func(cmd Command) {
		if cmd.Action != nil {
			cmd.Action()
		}
	})
}

func (a *App) getAvailableCommands() []Command {
	return []Command{
		{Name: "New File", Shortcut: "Ctrl+N", Icon: theme.DocumentCreateIcon(), Action: a.newFile},
		{Name: "Open File", Shortcut: "Ctrl+O", Icon: theme.FolderOpenIcon(), Action: a.openFile},
		{Name: "Save File", Shortcut: "Ctrl+S", Icon: theme.DocumentSaveIcon(), Action: a.saveFile},
		{Name: "Find", Shortcut: "Ctrl+F", Icon: theme.SearchIcon(), Action: a.showFind},
		{Name: "Replace", Shortcut: "Ctrl+H", Icon: theme.SearchReplaceIcon(), Action: a.showReplace},
		{Name: "Toggle Sidebar", Shortcut: "Ctrl+B", Icon: theme.MenuIcon(), Action: a.toggleSidebar}, // Исправлено: заменено ViewListIcon на MenuIcon
		{Name: "Toggle Minimap", Shortcut: "Ctrl+M", Icon: theme.ViewFullScreenIcon(), Action: a.toggleMinimap},
		{Name: "Format Code", Shortcut: "Shift+Alt+F", Icon: theme.DocumentIcon(), Action: a.formatCode},
		{Name: "Preferences", Shortcut: "", Icon: theme.SettingsIcon(), Action: a.showPreferences},
		{Name: "About", Shortcut: "", Icon: theme.InfoIcon(), Action: a.showAbout},
	}
}

func (a *App) focusFileExplorer() {
	// Фокусируемся на сайдбаре
	if a.sidebar != nil && a.sidebar.IsVisible() {
		// Делаем сайдбар активным
		if a.mainWin != nil && a.sidebar.fileTree != nil {
			a.mainWin.Canvas().Focus(a.sidebar.fileTree)
		}
	} else if a.sidebar != nil {
		// Если сайдбар скрыт, показываем его
		a.toggleSidebar()
		// И фокусируемся
		if a.mainWin != nil && a.sidebar.fileTree != nil {
			a.mainWin.Canvas().Focus(a.sidebar.fileTree)
		}
	}
}

func (a *App) zoomIn() {
	if a.editor == nil || a.config == nil {
		return
	}

	a.config.Editor.FontSize += 2
	if a.config.Editor.FontSize > 72 {
		a.config.Editor.FontSize = 72
	}

	// Применяем размер шрифта к редактору
	a.applyFontSize()
	a.configManager.SaveConfig()
}

func (a *App) zoomOut() {
	if a.editor == nil || a.config == nil {
		return
	}

	a.config.Editor.FontSize -= 2
	if a.config.Editor.FontSize < 6 {
		a.config.Editor.FontSize = 6
	}

	// Применяем размер шрифта к редактору
	a.applyFontSize()
	a.configManager.SaveConfig()
}

func (a *App) resetZoom() {
	if a.editor == nil || a.config == nil {
		return
	}

	a.config.Editor.FontSize = 14

	// Применяем размер шрифта к редактору
	a.applyFontSize()
	a.configManager.SaveConfig()
}

// Tool operations - Полная реализация

func (a *App) openCMD() {
	workingDir := a.getCurrentWorkingDir()
	err := a.terminalMgr.OpenCMD(workingDir)
	if err != nil {
		dialog.ShowError(err, a.mainWin)
	}
}

func (a *App) openPowerShell() {
	workingDir := a.getCurrentWorkingDir()
	err := a.terminalMgr.OpenPowerShell(workingDir)
	if err != nil {
		dialog.ShowError(err, a.mainWin)
	}
}

func (a *App) showCustomTools() {
	if len(a.config.ExternalTools.CustomTools) == 0 {
		dialog.ShowInformation("Custom Tools", "No custom tools configured", a.mainWin)
		return
	}

	// Создаем список инструментов
	toolNames := make([]string, len(a.config.ExternalTools.CustomTools))
	for i, tool := range a.config.ExternalTools.CustomTools {
		toolNames[i] = tool.DisplayName
	}

	toolList := widget.NewList(
		func() int { return len(toolNames) },
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewIcon(theme.ComputerIcon()),
				widget.NewLabel("Tool"),
				widget.NewButton("Run", nil),
			)
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			box := o.(*fyne.Container)
			if len(box.Objects) >= 3 {
				label, ok1 := box.Objects[1].(*widget.Label)
				button, ok2 := box.Objects[2].(*widget.Button)

				if ok1 && ok2 {
					label.SetText(toolNames[i])
					button.OnTapped = func() {
						a.runCustomTool(a.config.ExternalTools.CustomTools[i])
					}
				}
			}
		},
	)

	toolDialog := dialog.NewCustom("Custom Tools", "Close",
		container.NewScroll(toolList), a.mainWin)
	toolDialog.Resize(fyne.NewSize(500, 400))
	toolDialog.Show()
}

func (a *App) formatCode() {
	if a.editor == nil || a.currentFile == "" {
		return
	}

	// Определяем язык по расширению
	ext := filepath.Ext(a.currentFile)
	language := getLanguageByExtension(ext)

	// Форматируем код
	cmd := &FormatCodeCommand{
		language: language,
	}

	err := a.commandHistory.Execute(cmd, a.editor)
	if err != nil {
		dialog.ShowError(err, a.mainWin)
	}
}

func (a *App) lintCode() {
	if a.editor == nil || a.currentFile == "" {
		return
	}

	// Определяем язык по расширению
	ext := filepath.Ext(a.currentFile)
	language := getLanguageByExtension(ext)

	// Получаем конфигурацию линтера
	linterConfig, exists := a.config.ExternalTools.Linters.LanguageLinters[language]
	if !exists || !linterConfig.Enabled {
		dialog.ShowInformation("Lint", fmt.Sprintf("No linter configured for %s files", language), a.mainWin)
		return
	}

	// Сохраняем файл перед линтингом
	a.saveFile()

	// Запускаем линтер
	output, err := a.terminalMgr.RunCommand(
		fmt.Sprintf("%s %s %s", linterConfig.Path, strings.Join(linterConfig.Args, " "), a.currentFile),
		filepath.Dir(a.currentFile),
	)

	if err != nil && output == "" {
		dialog.ShowError(fmt.Errorf("Failed to run linter: %v", err), a.mainWin)
		return
	}

	// Парсим ошибки
	analyzer := NewCodeAnalyzer()
	errors := analyzer.FindErrors(output, language)

	if len(errors) == 0 {
		dialog.ShowInformation("Lint", "No issues found!", a.mainWin)
	} else {
		// Показываем результаты линтинга
		a.showLintResults(errors)
	}
}

func (a *App) runFile() {
	if a.currentFile == "" {
		dialog.ShowInformation("No File", "Please open a file first", a.mainWin)
		return
	}

	// Определяем язык и команду запуска
	ext := filepath.Ext(a.currentFile)
	language := getLanguageByExtension(ext)

	var runCommand string
	switch language {
	case "go":
		runCommand = fmt.Sprintf("go run %s", a.currentFile)
	case "python":
		runCommand = fmt.Sprintf("python %s", a.currentFile)
	case "rust":
		// Для Rust сначала компилируем
		runCommand = fmt.Sprintf("rustc %s && %s", a.currentFile,
			strings.TrimSuffix(a.currentFile, ext))
	case "java":
		className := strings.TrimSuffix(filepath.Base(a.currentFile), ext)
		runCommand = fmt.Sprintf("javac %s && java %s", a.currentFile, className)
	case "c":
		outputFile := strings.TrimSuffix(a.currentFile, ext) + ".exe"
		runCommand = fmt.Sprintf("gcc %s -o %s && %s", a.currentFile, outputFile, outputFile)
	default:
		dialog.ShowInformation("Run File",
			fmt.Sprintf("Don't know how to run %s files", language), a.mainWin)
		return
	}

	// Открываем терминал и выполняем команду
	terminal, err := a.terminalMgr.OpenTerminal(TerminalPowerShell, filepath.Dir(a.currentFile))
	if err != nil {
		dialog.ShowError(err, a.mainWin)
		return
	}

	// Отправляем команду
	a.terminalMgr.sendCommand(terminal, runCommand)
}

func (a *App) buildProject() {
	if a.currentFile == "" {
		dialog.ShowInformation("No Project", "Please open a project file first", a.mainWin)
		return
	}

	projectDir := filepath.Dir(a.currentFile)

	// Определяем тип проекта по файлам в директории
	var buildCommand string

	if _, err := os.Stat(filepath.Join(projectDir, "go.mod")); err == nil {
		// Go проект
		buildCommand = "go build ."
	} else if _, err := os.Stat(filepath.Join(projectDir, "Cargo.toml")); err == nil {
		// Rust проект
		buildCommand = "cargo build"
	} else if _, err := os.Stat(filepath.Join(projectDir, "package.json")); err == nil {
		// Node.js проект
		buildCommand = "npm build"
	} else if _, err := os.Stat(filepath.Join(projectDir, "pom.xml")); err == nil {
		// Maven проект
		buildCommand = "mvn compile"
	} else if _, err := os.Stat(filepath.Join(projectDir, "Makefile")); err == nil {
		// Makefile проект
		buildCommand = "make"
	} else {
		dialog.ShowInformation("Build Project",
			"Cannot determine project type. No build file found.", a.mainWin)
		return
	}

	// Открываем терминал и выполняем сборку
	terminal, err := a.terminalMgr.OpenTerminal(TerminalPowerShell, projectDir)
	if err != nil {
		dialog.ShowError(err, a.mainWin)
		return
	}

	// Отправляем команду сборки
	a.terminalMgr.sendCommand(terminal, buildCommand)
}

// Settings operations

func (a *App) showPreferences() {
	a.dialogManager.ShowPreferencesDialog(a.config, func(config *Config) {
		// Сохраняем настройки
		a.config = config
		a.configManager.UpdateConfig(func(c *Config) {
			*c = *config
		})

		// Применяем изменения
		a.applyConfigChanges()
	})
}

func (a *App) showKeyBindings() {
	// Создаем диалог управления горячими клавишами
	shortcuts := a.hotkeyManager.GetShortcuts()

	// Группируем по категориям
	categories := make(map[string][]*RegisteredShortcut)
	for _, shortcut := range shortcuts {
		categories[shortcut.Category] = append(categories[shortcut.Category], shortcut)
	}

	// Создаем вкладки для каждой категории
	tabs := container.NewAppTabs()

	for category, categoryShortcuts := range categories {
		// Создаем список для категории
		list := widget.NewList(
			func() int { return len(categoryShortcuts) },
			func() fyne.CanvasObject {
				return container.NewHBox(
					widget.NewLabel("Action"),
					widget.NewLabel("Shortcut"),
					widget.NewButton("Change", nil),
				)
			},
			func(i widget.ListItemID, o fyne.CanvasObject) {
				box := o.(*fyne.Container)
				actionLabel := box.Objects[0].(*widget.Label)
				shortcutLabel := box.Objects[1].(*widget.Label)
				button := box.Objects[2].(*widget.Button)

				shortcut := categoryShortcuts[i]
				actionLabel.SetText(shortcut.Description)
				shortcutLabel.SetText(a.hotkeyManager.shortcutToString(shortcut.Shortcut))

				button.OnTapped = func() {
					a.changeKeyBinding(shortcut)
				}
			},
		)

		tabs.Append(container.NewTabItem(category, list))
	}

	keyDialog := dialog.NewCustom("Key Bindings", "Close", tabs, a.mainWin)
	keyDialog.Resize(fyne.NewSize(700, 500))
	keyDialog.Show()
}

func (a *App) showAbout() {
	a.dialogManager.ShowAboutDialog()
}

func (a *App) showRecentFiles() {
	if len(a.recentFiles) == 0 {
		dialog.ShowInformation("Recent Files", "No recent files", a.mainWin)
		return
	}

	// Создаем список недавних файлов
	fileList := widget.NewList(
		func() int { return len(a.recentFiles) },
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewIcon(theme.DocumentIcon()),
				widget.NewLabel("File"),
				widget.NewButton("Open", nil),
			)
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			box := o.(*fyne.Container)
			if len(box.Objects) >= 3 {
				label, ok1 := box.Objects[1].(*widget.Label)
				button, ok2 := box.Objects[2].(*widget.Button)

				if ok1 && ok2 {
					label.SetText(filepath.Base(a.recentFiles[i]))
					button.OnTapped = func() {
						recentFile := a.recentFiles[i]
						a.loadFile(recentFile)
					}
				}
			}
		},
	)

	recentDialog := dialog.NewCustom("Recent Files", "Close",
		container.NewScroll(fileList), a.mainWin)
	recentDialog.Resize(fyne.NewSize(600, 400))
	recentDialog.Show()
}

// Utility methods - Полная реализация

func (a *App) getCurrentWorkingDir() string {
	if a.currentFile != "" {
		return filepath.Dir(a.currentFile)
	}

	if a.sidebar != nil {
		return a.sidebar.GetCurrentPath()
	}

	dir, _ := os.Getwd()
	return dir
}

func (a *App) updateTitle() {
	title := "Programmer's Notepad"
	if a.currentFile != "" {
		title = fmt.Sprintf("%s - %s", filepath.Base(a.currentFile), title)
		if a.editor != nil && a.editor.IsDirty() {
			title = "* " + title
		}
	}
	a.mainWin.SetTitle(title)
}

func (a *App) addToRecentFiles(filepath string) {
	// Удаляем если уже есть в списке
	for i, file := range a.recentFiles {
		if file == filepath {
			a.recentFiles = append(a.recentFiles[:i], a.recentFiles[i+1:]...)
			break
		}
	}

	// Добавляем в начало
	a.recentFiles = append([]string{filepath}, a.recentFiles...)

	// Ограничиваем размер
	if len(a.recentFiles) > 10 {
		a.recentFiles = a.recentFiles[:10]
	}

	// Сохраняем в конфигурации
	a.config.App.LastOpenedFiles = a.recentFiles
	a.configManager.SaveConfig()
}

func (a *App) applyConfigChanges() {
	// Применяем тему
	if a.config.App.Theme == "dark" {
		a.fyneApp.Settings().SetTheme(&DarkTheme{})
	} else if a.config.App.Theme == "light" {
		a.fyneApp.Settings().SetTheme(&LightTheme{})
	}

	// Применяем настройки редактора
	if a.editor != nil {
		a.editor.config = a.config
		a.editor.colors = GetEditorColors(a.config.App.Theme == "dark")
		a.editor.setupSyntaxHighlighter()
		a.editor.updateDisplay()
	}

	// Применяем настройки сайдбара
	if a.sidebar != nil {
		a.sidebar.config = a.config
		a.sidebar.showHiddenFiles = a.config.Sidebar.ShowHiddenFiles
		a.sidebar.sortBy = getSortType(a.config.Sidebar.SortBy)
		a.sidebar.sortAscending = a.config.Sidebar.SortAscending
		a.sidebar.applyFilterAndSearch()
	}

	// Применяем настройки миниатюры
	if a.minimap != nil {
		a.minimap.SetShowSyntax(a.config.Minimap.ShowSyntax)
		a.minimap.SetShowLineNumbers(a.config.Minimap.ShowLineNumbers)
		a.minimap.SetSmoothScrolling(a.config.Minimap.SmoothScrolling)
		a.minimap.SetWidth(a.config.Minimap.Width)
		a.minimap.Refresh()
	}
}

func (a *App) checkAndExit() {
	if a.editor != nil && a.editor.IsDirty() {
		dialog.ShowConfirm("Unsaved Changes",
			"Do you want to save before exiting?",
			func(save bool) {
				if save {
					a.saveFile()
				}
				a.cleanup()
				a.fyneApp.Quit()
			}, a.mainWin)
	} else {
		a.cleanup()
		a.fyneApp.Quit()
	}
}

func (a *App) cleanup() {
	// Сохраняем позицию и размер окна
	size := a.mainWin.Canvas().Size()
	a.config.App.WindowWidth = int(size.Width)
	a.config.App.WindowHeight = int(size.Height)
	a.configManager.SaveConfig()

	// Закрываем все терминалы
	if a.terminalMgr != nil {
		a.terminalMgr.CloseAllTerminals()
	}

	// Очищаем ресурсы компонентов
	if a.sidebar != nil {
		a.sidebar.Cleanup()
	}

	if a.minimap != nil {
		a.minimap.Cleanup()
	}

	if a.hotkeyManager != nil {
		a.hotkeyManager.Cleanup()
	}

	if a.configManager != nil {
		a.configManager.Cleanup()
	}
}

// Дополнительные вспомогательные методы

func (a *App) getSelectedText() string {
	if a.editor == nil {
		return ""
	}

	start := a.editor.selectionStart
	end := a.editor.selectionEnd

	if start.Row == end.Row && start.Col == end.Col {
		return ""
	}

	lines := strings.Split(a.editor.textContent, "\n")

	if start.Row == end.Row {
		// Выделение в одной строке
		if start.Row < len(lines) {
			line := lines[start.Row]
			if start.Col < len(line) && end.Col <= len(line) {
				return line[start.Col:end.Col]
			}
		}
	} else {
		// Многострочное выделение
		var selected strings.Builder
		for i := start.Row; i <= end.Row && i < len(lines); i++ {
			line := lines[i]
			if i == start.Row {
				selected.WriteString(line[start.Col:])
			} else if i == end.Row {
				selected.WriteString(line[:end.Col])
			} else {
				selected.WriteString(line)
			}
			if i < end.Row {
				selected.WriteString("\n")
			}
		}
		return selected.String()
	}

	return ""
}

func (a *App) deleteSelectedText() {
	if a.editor == nil {
		return
	}

	cmd := &DeleteTextCommand{
		startPos: a.editor.selectionStart,
		endPos:   a.editor.selectionEnd,
	}

	a.commandHistory.Execute(cmd, a.editor)
}

func (a *App) goToSearchResult(index int) {
	if index < 0 || index >= len(a.searchResults) {
		return
	}

	result := a.searchResults[index]
	a.editor.cursorRow = result.Start.Row
	a.editor.cursorCol = result.Start.Col

	// Выделяем найденный текст
	a.editor.selectionStart = result.Start
	a.editor.selectionEnd = result.End

	a.editor.updateDisplay()
}

func (a *App) replaceCurrentMatch(find, replace string) {
	if a.editor == nil || len(a.searchResults) == 0 {
		return
	}

	// Заменяем текущее совпадение
	result := a.searchResults[a.currentSearchIndex]

	// Создаем команду замены для конкретного места
	lines := strings.Split(a.editor.textContent, "\n")
	if result.Start.Row < len(lines) {
		line := lines[result.Start.Row]
		newLine := line[:result.Start.Col] + replace + line[result.End.Col:]
		lines[result.Start.Row] = newLine

		oldContent := a.editor.textContent
		a.editor.textContent = strings.Join(lines, "\n")

		// Добавляем в историю
		cmd := &ReplaceTextCommand{
			findText:    find,
			replaceText: replace,
			oldText:     oldContent,
		}
		cmd.oldText = oldContent
		a.commandHistory.commands = append(a.commandHistory.commands, cmd)
		a.commandHistory.currentIndex++

		a.editor.updateDisplay()
	}
}

func (a *App) performFindInFiles(searchText, path, include, exclude string) {
	// Здесь должна быть реализация поиска в файлах
	// Пока показываем заглушку
	dialog.ShowInformation("Find in Files",
		fmt.Sprintf("Searching for '%s' in %s", searchText, path), a.mainWin)
}

func (a *App) showLintResults(errors []CompilerError) {
	// Создаем список ошибок
	errorList := widget.NewList(
		func() int { return len(errors) },
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewIcon(theme.ErrorIcon()),
				widget.NewLabel("Error"),
			)
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			box := o.(*fyne.Container)
			icon := box.Objects[0].(*widget.Icon)
			label := box.Objects[1].(*widget.Label)

			err := errors[i]
			if err.Type == "warning" {
				icon.SetResource(theme.WarningIcon())
			}

			label.SetText(fmt.Sprintf("Line %d: %s", err.Line, err.Message))
		},
	)

	errorList.OnSelected = func(id widget.ListItemID) {
		if id < len(errors) {
			a.goToLine(errors[id].Line)
		}
	}

	lintDialog := dialog.NewCustom("Lint Results", "Close",
		container.NewScroll(errorList), a.mainWin)
	lintDialog.Resize(fyne.NewSize(600, 400))
	lintDialog.Show()
}

func (a *App) runCustomTool(tool CustomTool) {
	if !tool.Enabled {
		dialog.ShowInformation("Tool Disabled",
			fmt.Sprintf("Tool '%s' is disabled", tool.DisplayName), a.mainWin)
		return
	}

	workingDir := tool.WorkingDirectory
	if workingDir == "" {
		workingDir = a.getCurrentWorkingDir()
	}

	// Заменяем переменные в аргументах
	args := tool.Args
	args = strings.ReplaceAll(args, "${file}", a.currentFile)
	args = strings.ReplaceAll(args, "${dir}", filepath.Dir(a.currentFile))
	args = strings.ReplaceAll(args, "${name}", filepath.Base(a.currentFile))

	command := fmt.Sprintf("%s %s", tool.Path, args)

	if tool.RunInTerminal {
		// Запускаем в терминале
		terminal, err := a.terminalMgr.OpenTerminal(TerminalPowerShell, workingDir)
		if err != nil {
			dialog.ShowError(err, a.mainWin)
			return
		}
		a.terminalMgr.sendCommand(terminal, command)
	} else {
		// Запускаем в фоне
		output, err := a.terminalMgr.RunCommand(command, workingDir)
		if err != nil {
			dialog.ShowError(err, a.mainWin)
		} else if tool.ShowOutput && output != "" {
			dialog.ShowInformation(tool.DisplayName, output, a.mainWin)
		}
	}
}

func (a *App) applyFontSize() {
	if a.editor == nil || a.editor.content == nil || a.appTheme == nil {
		return
	}

	// Обновляем размер шрифта через тему
	a.appTheme.SetFontSize(a.config.Editor.FontSize)
	a.fyneApp.Settings().SetTheme(a.appTheme)
	a.editor.content.Refresh()

	// Обновляем номера строк
	if a.editor.lineNumbers != nil {
		a.editor.lineNumbers.Refresh()
	}
}

func (a *App) updateStatusBar(row, col int) {
	if a.statusBar != nil && a.editor != nil {
		totalLines := a.editor.getLineCount()
		status := fmt.Sprintf("Line %d, Column %d | Total Lines: %d", row+1, col+1, totalLines)
		a.statusBar.SetText(status)
	}
}

func (a *App) scrollEditorTo(position float32) {
	if a.editor == nil || a.editor.scrollContainer == nil {
		return
	}

	// Прокручиваем редактор к указанной позиции
	contentHeight := float32(a.editor.getLineCount()) * 20 // Примерная высота строки
	scrollY := position * contentHeight

	a.editor.scrollContainer.Offset.Y = scrollY
	a.editor.scrollContainer.Refresh()
}

func (a *App) changeKeyBinding(shortcut *RegisteredShortcut) {
	// Диалог изменения горячей клавиши
	entry := widget.NewEntry()
	entry.SetPlaceHolder("Press new key combination...")

	dialog.ShowCustomConfirm("Change Key Binding", "Save", "Cancel",
		container.NewVBox(
			widget.NewLabel(shortcut.Description),
			widget.NewLabel("Current: "+a.hotkeyManager.shortcutToString(shortcut.Shortcut)),
			entry,
		), func(save bool) {
			if save && entry.Text != "" {
				err := a.hotkeyManager.UpdateKeyBinding(shortcut.ID, entry.Text)
				if err != nil {
					dialog.ShowError(err, a.mainWin)
				}
			}
		}, a.mainWin)
}

// Helper functions

func getLanguageByExtension(ext string) string {
	switch strings.ToLower(ext) {
	case ".go":
		return "go"
	case ".rs":
		return "rust"
	case ".py":
		return "python"
	case ".c", ".h":
		return "c"
	case ".java":
		return "java"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	default:
		return "text"
	}
}

func getSortType(sortBy string) SortType {
	switch sortBy {
	case "name":
		return SortByName
	case "type":
		return SortByType
	case "size":
		return SortBySize
	case "date":
		return SortByDate
	default:
		return SortByName
	}
}

// Run запускает приложение
func (a *App) Run() {
	a.setupUI()

	// Обработчик закрытия окна
	a.mainWin.SetCloseIntercept(func() {
		a.checkAndExit()
	})

	a.mainWin.ShowAndRun()
}

func main() {
	app := NewApp()
	app.Run()
}
