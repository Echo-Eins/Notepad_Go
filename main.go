package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// App представляет основное приложение
type App struct {
	fyneApp fyne.App
	mainWin fyne.Window
	editor  *EditorWidget
	sidebar *SidebarWidget
	minimap *MinimapWidget
	config  *Config
}

// EditorWidget - основной виджет редактора
type EditorWidget struct {
	widget.RichText
	filePath    string
	isDirty     bool
	content     string
	lineNumbers *widget.List
}

// SidebarWidget - боковая панель с файлами
type SidebarWidget struct {
	widget.List
	currentPath string
	files       []FileItem
	isVisible   bool
}

// MinimapWidget - миниатюрная карта кода
type MinimapWidget struct {
	widget.List
	editor *EditorWidget
	lines  []string
}

// FileItem представляет файл или папку
type FileItem struct {
	Name     string
	Path     string
	IsDir    bool
	Size     int64
	Modified string
}

// Config хранит настройки приложения
type Config struct {
	Theme           string            `json:"theme"`
	FontFamily      string            `json:"font_family"`
	FontSize        float32           `json:"font_size"`
	ShowLineNumbers bool              `json:"show_line_numbers"`
	ShowMinimap     bool              `json:"show_minimap"`
	WordWrap        bool              `json:"word_wrap"`
	IndentGuides    bool              `json:"indent_guides"`
	AutoSave        bool              `json:"auto_save"`
	AutoSaveMinutes int               `json:"auto_save_minutes"`
	KeyBindings     map[string]string `json:"key_bindings"`
	TerminalDefault string            `json:"terminal_default"`
	CustomTools     []CustomTool      `json:"custom_tools"`
	TrackingWords   []string          `json:"tracking_words"`
}

// CustomTool представляет пользовательский инструмент
type CustomTool struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Args string `json:"args"`
}

// NewApp создает новое приложение
func NewApp() *App {
	myApp := app.NewWithID("dev.notepad.programmer")
	myApp.SetIcon(theme.DocumentIcon())

	// Загружаем конфигурацию
	config := LoadConfig()

	// Устанавливаем тему
	if config.Theme == "dark" {
		myApp.Settings().SetTheme(&DarkTheme{})
	} else {
		myApp.Settings().SetTheme(theme.DefaultTheme())
	}

	mainWin := myApp.NewWindow("Programmer's Notepad")
	mainWin.Resize(fyne.NewSize(1400, 900))

	return &App{
		fyneApp: myApp,
		mainWin: mainWin,
		config:  config,
	}
}

// setupUI создает и настраивает пользовательский интерфейс
func (a *App) setupUI() {
	// Создаем основные компоненты
	a.editor = NewEditor(a.config)
	a.sidebar = NewSidebar(a.config)
	a.minimap = NewMinimap(a.editor)

	// Создаем меню
	fileMenu := fyne.NewMenu("File",
		fyne.NewMenuItem("New", a.newFile),
		fyne.NewMenuItem("Open", a.openFile),
		fyne.NewMenuItem("Save", a.saveFile),
		fyne.NewMenuItem("Save As", a.saveAsFile),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Exit", func() { a.fyneApp.Quit() }),
	)

	editMenu := fyne.NewMenu("Edit",
		fyne.NewMenuItem("Find", a.showFind),
		fyne.NewMenuItem("Replace", a.showReplace),
		fyne.NewMenuItem("Go to Line", a.showGoToLine),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Select All", a.selectAll),
	)

	viewMenu := fyne.NewMenu("View",
		fyne.NewMenuItem("Toggle Sidebar", a.toggleSidebar),
		fyne.NewMenuItem("Toggle Minimap", a.toggleMinimap),
		fyne.NewMenuItem("Command Palette", a.showCommandPalette),
	)

	toolsMenu := fyne.NewMenu("Tools",
		fyne.NewMenuItem("Terminal (CMD)", a.openCMD),
		fyne.NewMenuItem("Terminal (PowerShell)", a.openPowerShell),
		fyne.NewMenuItem("Custom Tools", a.showCustomTools),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Format Code", a.formatCode),
		fyne.NewMenuItem("Lint Code", a.lintCode),
	)

	settingsMenu := fyne.NewMenu("Settings",
		fyne.NewMenuItem("Preferences", a.showPreferences),
		fyne.NewMenuItem("Key Bindings", a.showKeyBindings),
		fyne.NewMenuItem("About", a.showAbout),
	)

	mainMenu := fyne.NewMainMenu(fileMenu, editMenu, viewMenu, toolsMenu, settingsMenu)
	a.mainWin.SetMainMenu(mainMenu)

	// Создаем основной layout
	var mainContent *container.Border

	if a.config.ShowMinimap {
		// Редактор + миниатюрная карта
		editorWithMinimap := container.NewBorder(nil, nil, nil, a.minimap, a.editor)
		mainContent = container.NewBorder(nil, nil, a.sidebar, nil, editorWithMinimap)
	} else {
		mainContent = container.NewBorder(nil, nil, a.sidebar, nil, a.editor)
	}

	a.mainWin.SetContent(mainContent)

	// Устанавливаем горячие клавиши
	a.setupHotkeys()
}

// Заглушки для методов (будем реализовывать поэтапно)
func (a *App) newFile()            {}
func (a *App) openFile()           {}
func (a *App) saveFile()           {}
func (a *App) saveAsFile()         {}
func (a *App) showFind()           {}
func (a *App) showReplace()        {}
func (a *App) showGoToLine()       {}
func (a *App) selectAll()          {}
func (a *App) toggleSidebar()      {}
func (a *App) toggleMinimap()      {}
func (a *App) showCommandPalette() {}
func (a *App) openCMD()            {}
func (a *App) openPowerShell()     {}
func (a *App) showCustomTools()    {}
func (a *App) formatCode()         {}
func (a *App) lintCode()           {}
func (a *App) showPreferences()    {}
func (a *App) showKeyBindings()    {}
func (a *App) showAbout()          {}
func (a *App) setupHotkeys()       {}

// Заглушки для конструкторов (будем реализовывать поэтапно)
func NewEditor(config *Config) *EditorWidget   { return &EditorWidget{} }
func NewSidebar(config *Config) *SidebarWidget { return &SidebarWidget{} }
func NewMinimap(editor *EditorWidget) *MinimapWidget {
	return &MinimapWidget{editor: editor}
}
func LoadConfig() *Config {
	return &Config{
		Theme:           "dark",
		FontFamily:      "JetBrains Mono",
		FontSize:        14,
		ShowLineNumbers: true,
		ShowMinimap:     true,
		WordWrap:        false,
		IndentGuides:    true,
		AutoSave:        true,
		AutoSaveMinutes: 5,
		KeyBindings:     make(map[string]string),
		TerminalDefault: "powershell",
		TrackingWords:   []string{"TODO", "FIXME", "NOTE", "HACK", "BUG"},
	}
}

// Run запускает приложение
func (a *App) Run() {
	a.setupUI()
	a.mainWin.ShowAndRun()
}

func main() {
	app := NewApp()
	app.Run()
}
