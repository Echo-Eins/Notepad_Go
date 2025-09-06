package main

import (
	"fmt"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// App представляет основное приложение
type App struct {
	fyneApp       fyne.App
	mainWin       fyne.Window
	editor        *EditorWidget
	sidebar       *SidebarWidget
	minimap       *MinimapWidget
	config        *Config
	configManager *ConfigManager
	hotkeyManager *HotkeyManager
	dialogManager *DialogManager
	terminalMgr   *TerminalManager
	mainContent   *container.Border
	currentFile   string
	recentFiles   []string
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

	// Устанавливаем тему
	if config.App.Theme == "dark" {
		myApp.Settings().SetTheme(&DarkTheme{})
	} else if config.App.Theme == "light" {
		myApp.Settings().SetTheme(&LightTheme{})
	} else {
		myApp.Settings().SetTheme(&DarkTheme{})
	}

	mainWin := myApp.NewWindow("Programmer's Notepad")

	// Восстанавливаем размер и позицию окна
	mainWin.Resize(fyne.NewSize(float32(config.App.WindowWidth), float32(config.App.WindowHeight)))
	if config.App.WindowMaximized {
		mainWin.SetFullScreen(true)
	}

	appInstance := &App{
		fyneApp:       myApp,
		mainWin:       mainWin,
		config:        config,
		configManager: configMgr,
		recentFiles:   config.App.LastOpenedFiles,
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
		a.openFile(a.recentFiles[0])
	}

	// Устанавливаем начальную директорию для сайдбара
	homeDir, _ := os.UserHomeDir()
	a.sidebar.SetRootPath(homeDir)
}

// createMainMenu создает главное меню
func (a *App) createMainMenu() {
	fileMenu := fyne.NewMenu("File",
		fyne.NewMenuItem("New", a.newFile),
		fyne.NewMenuItem("Open...", a.openFile),
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
	statusBar := a.createStatusBar()

	// Основной контент с редактором и миниатюрой
	var editorContent fyne.CanvasObject
	if a.config.Minimap.IsVisible {
		editorContent = container.NewBorder(nil, nil, nil, a.minimap, a.editor)
	} else {
		editorContent = a.editor
	}

	// Добавляем боковую панель если видима
	if a.config.Sidebar.IsVisible {
		a.mainContent = container.NewBorder(nil, statusBar, a.sidebar, nil, editorContent)
	} else {
		a.mainContent = container.NewBorder(nil, statusBar, nil, nil, editorContent)
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

	// Язык/режим
	modeLabel := widget.NewLabel("Plain Text")

	// Кодировка
	encodingLabel := widget.NewLabel("UTF-8")

	// Разделители
	sep1 := widget.NewSeparator()
	sep2 := widget.NewSeparator()
	sep3 := widget.NewSeparator()

	statusBar := container.NewHBox(
		fileLabel,
		sep1,
		positionLabel,
		sep2,
		modeLabel,
		sep3,
		encodingLabel,
	)

	return statusBar
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
			// TODO: Update status bar position
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
				a.openFile(path)
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
				// TODO: Implement editor scrolling
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

func (a *App) openFile(paths ...string) {
	if len(paths) > 0 {
		// Открываем указанный файл
		a.loadFile(paths[0])
	} else {
		// Показываем диалог выбора файла
		a.dialogManager.ShowOpenFileDialog(func(path string) {
			a.loadFile(path)
		})
	}
}

func (a *App) loadFile(path string) {
	if a.editor != nil {
		err := a.editor.LoadFile(path)
		if err != nil {
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

// Edit operations

func (a *App) undo() {
	// TODO: Implement undo in editor
	fmt.Println("Undo")
}

func (a *App) redo() {
	// TODO: Implement redo in editor
	fmt.Println("Redo")
}

func (a *App) cut() {
	// TODO: Implement cut in editor
	fmt.Println("Cut")
}

func (a *App) copy() {
	// TODO: Implement copy in editor
	fmt.Println("Copy")
}

func (a *App) paste() {
	// TODO: Implement paste in editor
	fmt.Println("Paste")
}

func (a *App) selectAll() {
	// TODO: Implement select all in editor
	fmt.Println("Select All")
}

// Search operations

func (a *App) showFind() {
	a.dialogManager.ShowFindDialog(func(text string, caseSensitive, wholeWord, regex bool) {
		// TODO: Implement find in editor
		fmt.Printf("Find: %s (case:%v, word:%v, regex:%v)\n", text, caseSensitive, wholeWord, regex)
	})
}

func (a *App) showReplace() {
	a.dialogManager.ShowReplaceDialog(func(find, replace string, caseSensitive, wholeWord, regex, replaceAll bool) {
		// TODO: Implement replace in editor
		fmt.Printf("Replace: %s -> %s (all:%v)\n", find, replace, replaceAll)
	})
}

func (a *App) showFindInFiles() {
	// TODO: Implement find in files dialog
	fmt.Println("Find in Files")
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
	if a.editor != nil {
		// TODO: Implement go to line in editor
		fmt.Printf("Go to line: %d\n", line)
	}
}

func (a *App) showGoToSymbol() {
	// TODO: Implement go to symbol
	fmt.Println("Go to Symbol")
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
		{Name: "Toggle Sidebar", Shortcut: "Ctrl+B", Icon: theme.ViewListIcon(), Action: a.toggleSidebar},
		{Name: "Toggle Minimap", Shortcut: "Ctrl+M", Icon: theme.ViewFullScreenIcon(), Action: a.toggleMinimap},
		{Name: "Format Code", Shortcut: "Shift+Alt+F", Icon: theme.DocumentIcon(), Action: a.formatCode},
		{Name: "Preferences", Shortcut: "", Icon: theme.SettingsIcon(), Action: a.showPreferences},
		{Name: "About", Shortcut: "", Icon: theme.InfoIcon(), Action: a.showAbout},
	}
}

func (a *App) focusFileExplorer() {
	// Focus on sidebar
	if a.sidebar != nil && a.sidebar.IsVisible() {
		// TODO: Implement focus on sidebar
		fmt.Println("Focus File Explorer")
	}
}

func (a *App) zoomIn() {
	if a.editor != nil && a.config != nil {
		a.config.Editor.FontSize += 2
		if a.config.Editor.FontSize > 72 {
			a.config.Editor.FontSize = 72
		}
		// TODO: Apply font size to editor
		a.configManager.SaveConfig()
	}
}

func (a *App) zoomOut() {
	if a.editor != nil && a.config != nil {
		a.config.Editor.FontSize -= 2
		if a.config.Editor.FontSize < 6 {
			a.config.Editor.FontSize = 6
		}
		// TODO: Apply font size to editor
		a.configManager.SaveConfig()
	}
}

func (a *App) resetZoom() {
	if a.editor != nil && a.config != nil {
		a.config.Editor.FontSize = 14
		// TODO: Apply font size to editor
		a.configManager.SaveConfig()
	}
}

// Tool operations

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
	// TODO: Implement custom tools dialog
	fmt.Println("Custom Tools")
}

func (a *App) formatCode() {
	if a.editor == nil || a.currentFile == "" {
		return
	}

	// TODO: Implement code formatting based on file type
	fmt.Println("Format Code")
}

func (a *App) lintCode() {
	if a.editor == nil || a.currentFile == "" {
		return
	}

	// TODO: Implement code linting based on file type
	fmt.Println("Lint Code")
}

func (a *App) runFile() {
	if a.currentFile == "" {
		dialog.ShowInformation("No File", "Please open a file first", a.mainWin)
		return
	}

	// TODO: Implement file execution based on type
	fmt.Printf("Run file: %s\n", a.currentFile)
}

func (a *App) buildProject() {
	// TODO: Implement project build
	fmt.Println("Build Project")
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
	// TODO: Implement key bindings dialog
	fmt.Println("Key Bindings")
}

func (a *App) showAbout() {
	a.dialogManager.ShowAboutDialog()
}

func (a *App) showRecentFiles() {
	if len(a.recentFiles) == 0 {
		dialog.ShowInformation("Recent Files", "No recent files", a.mainWin)
		return
	}

	// TODO: Implement recent files dialog
	fmt.Println("Recent Files:", a.recentFiles)
}

// Utility methods

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

	// Обновляем компоненты
	if a.editor != nil {
		// TODO: Apply editor settings
	}

	if a.sidebar != nil {
		// TODO: Apply sidebar settings
	}

	if a.minimap != nil {
		// TODO: Apply minimap settings
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
