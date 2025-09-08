package main

import (
	"context"
	"encoding/json"
	"fmt"
	_ "fyne.io/fyne/v2"
	_ "fyne.io/fyne/v2/storage"
	"github.com/fsnotify/fsnotify"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Config - главная структура настроек приложения
type Config struct {
	// Метаданные конфигурации
	Version      string    `json:"version"`
	LastModified time.Time `json:"last_modified"`
	ConfigPath   string    `json:"-"` // Не сохраняется в JSON

	// Основные настройки приложения
	App AppConfig `json:"app"`

	// Настройки редактора
	Editor EditorConfig `json:"editor"`

	// Настройки файлового менеджера
	Sidebar SidebarConfig `json:"sidebar"`

	// Настройки миниатюрной карты
	Minimap MinimapConfig `json:"minimap"`

	// Настройки горячих клавиш
	KeyBindings KeyBindingsConfig `json:"key_bindings"`

	// Настройки внешних инструментов
	ExternalTools ExternalToolsConfig `json:"external_tools"`

	// Настройки интеграции
	Integration IntegrationConfig `json:"integration"`

	// Расширенные настройки
	Advanced AdvancedConfig `json:"advanced"`
}

// AppConfig - основные настройки приложения
type AppConfig struct {
	Theme           string   `json:"theme"`    // "dark", "light", "auto"
	Language        string   `json:"language"` // "en", "ru"
	WindowWidth     int      `json:"window_width"`
	WindowHeight    int      `json:"window_height"`
	WindowX         int      `json:"window_x"`
	WindowY         int      `json:"window_y"`
	WindowMaximized bool     `json:"window_maximized"`
	LastOpenedFiles []string `json:"last_opened_files"`
	RecentProjects  []string `json:"recent_projects"`
	StartupBehavior string   `json:"startup_behavior"` // "blank", "last_session", "welcome"
	CheckUpdates    bool     `json:"check_updates"`
	CornerRadius    float32  `json:"corner_radius"`
}

// EditorConfig - настройки редактора кода
type EditorConfig struct {
	// Шрифты и отображение
	FontFamily string  `json:"font_family"`
	FontSize   float32 `json:"font_size"`
	LineHeight float32 `json:"line_height"`
	TabSize    int     `json:"tab_size"`
	IndentSize int     `json:"indent_size"`
	UseSpaces  bool    `json:"use_spaces"`

	// Визуальные элементы
	ShowLineNumbers     bool `json:"show_line_numbers"`
	ShowRelativeNumbers bool `json:"show_relative_numbers"`
	ShowRuler           bool `json:"show_ruler"`
	RulerColumn         int  `json:"ruler_column"`
	WordWrap            bool `json:"word_wrap"`
	WrapColumn          int  `json:"wrap_column"`

	// Отступы и направляющие
	IndentGuides         bool `json:"indent_guides"`
	ShowWhitespace       bool `json:"show_whitespace"`
	ShowEndOfLine        bool `json:"show_end_of_line"`
	HighlightCurrentLine bool `json:"highlight_current_line"`

	// Поведение редактора
	AutoIndent        bool `json:"auto_indent"`
	SmartIndent       bool `json:"smart_indent"`
	AutoCloseBrackets bool `json:"auto_close_brackets"`
	AutoCloseQuotes   bool `json:"auto_close_quotes"`
	AutoSurround      bool `json:"auto_surround"`

	// Подсветка и навигация
	SyntaxHighlighting    bool `json:"syntax_highlighting"`
	BracketMatching       bool `json:"bracket_matching"`
	HighlightMatches      bool `json:"highlight_matches"`
	HighlightCurrentWord  bool `json:"highlight_current_word"`
	WordHighlightDuration int  `json:"word_highlight_duration"`
	VariableHighlight     bool `json:"variable_highlight"`

	// Автосохранение
	AutoSave        bool   `json:"auto_save"`
	AutoSaveDelay   int    `json:"auto_save_delay"` // в секундах
	BackupFiles     bool   `json:"backup_files"`
	BackupDirectory string `json:"backup_directory"`

	// Форматирование кода
	FormatOnSave       bool `json:"format_on_save"`
	TrimWhitespace     bool `json:"trim_whitespace"`
	InsertFinalNewline bool `json:"insert_final_newline"`

	// Фолдинг кода
	CodeFolding  bool `json:"code_folding"`
	FoldComments bool `json:"fold_comments"`
	FoldImports  bool `json:"fold_imports"`

	// Поиск и замена
	SearchCaseSensitive bool `json:"search_case_sensitive"`
	SearchWholeWord     bool `json:"search_whole_word"`
	SearchRegex         bool `json:"search_regex"`
	SearchWrap          bool `json:"search_wrap"`

	// Vim режим
	VimMode     bool `json:"vim_mode"`
	VimShowMode bool `json:"vim_show_mode"`

	// Отслеживание изменений
	TrackingKeywords   []string `json:"tracking_keywords"`
	MaxFileSize        int64    `json:"max_file_size"` // в байтах
	SupportedEncodings []string `json:"supported_encodings"`
}

// SidebarConfig - настройки файлового менеджера
type SidebarConfig struct {
	// Отображение
	IsVisible bool    `json:"is_visible"`
	Width     float32 `json:"width"`
	Position  string  `json:"position"` // "left", "right"

	// Поведение
	ShowHiddenFiles bool `json:"show_hidden_files"`
	ShowGitIgnored  bool `json:"show_git_ignored"`
	ShowSystemFiles bool `json:"show_system_files"`

	// Сортировка и фильтрация
	SortBy        string `json:"sort_by"` // "name", "type", "size", "date"
	SortAscending bool   `json:"sort_ascending"`
	DefaultFilter string `json:"default_filter"` // "All", "Code", "Go", etc.

	// Дерево файлов
	ExpandDirectories bool `json:"expand_directories"`
	ShowFileIcons     bool `json:"show_file_icons"`
	ShowFileSize      bool `json:"show_file_size"`
	ShowModifiedDate  bool `json:"show_modified_date"`

	// Контекстное меню
	EnableContextMenu    bool `json:"enable_context_menu"`
	ShowInExplorer       bool `json:"show_in_explorer"`
	EnableFileOperations bool `json:"enable_file_operations"`

	// File watching
	EnableFileWatcher   bool `json:"enable_file_watcher"`
	WatchSubdirectories bool `json:"watch_subdirectories"`
	WatcherDelay        int  `json:"watcher_delay"` // в миллисекундах

	// Производительность
	MaxFilesToShow int  `json:"max_files_to_show"`
	LazyLoading    bool `json:"lazy_loading"`
	CacheDirectory bool `json:"cache_directory"`
}

// MinimapConfig - настройки миниатюрной карты
type MinimapConfig struct {
	// Отображение
	IsVisible bool    `json:"is_visible"`
	Width     float32 `json:"width"`
	Position  string  `json:"position"` // "left", "right"

	// Содержимое
	ShowSyntax       bool `json:"show_syntax"`
	ShowLineNumbers  bool `json:"show_line_numbers"`
	ShowIndentGuides bool `json:"show_indent_guides"`
	ShowScrollbar    bool `json:"show_scrollbar"`

	// Размеры и масштаб
	LineHeight      float32 `json:"line_height"`
	CharWidth       float32 `json:"char_width"`
	FontSize        float32 `json:"font_size"`
	MaxCharsPerLine int     `json:"max_chars_per_line"`

	// Поведение
	SmoothScrolling bool `json:"smooth_scrolling"`
	ClickToScroll   bool `json:"click_to_scroll"`
	AutoHide        bool `json:"auto_hide"`
	AutoHideDelay   int  `json:"auto_hide_delay"` // в секундах

	// Viewport indicator
	ShowViewport    bool    `json:"show_viewport"`
	ViewportOpacity float32 `json:"viewport_opacity"`
	ViewportColor   string  `json:"viewport_color"`

	// Производительность
	RenderFPS     int  `json:"render_fps"`
	EnableCaching bool `json:"enable_caching"`
	MaxCacheSize  int  `json:"max_cache_size"`
}

// KeyBindingsConfig - настройки горячих клавиш
type KeyBindingsConfig struct {
	// Файловые операции
	NewFile    string `json:"new_file"`
	OpenFile   string `json:"open_file"`
	SaveFile   string `json:"save_file"`
	SaveAsFile string `json:"save_as_file"`
	CloseFile  string `json:"close_file"`
	CloseAll   string `json:"close_all"`

	// Редактирование
	Cut       string `json:"cut"`
	Copy      string `json:"copy"`
	Paste     string `json:"paste"`
	SelectAll string `json:"select_all"`
	Undo      string `json:"undo"`
	Redo      string `json:"redo"`

	// Поиск и навигация
	Find           string `json:"find"`
	FindNext       string `json:"find_next"`
	FindPrevious   string `json:"find_previous"`
	Replace        string `json:"replace"`
	FindInFiles    string `json:"find_in_files"`
	GoToLine       string `json:"go_to_line"`
	GoToSymbol     string `json:"go_to_symbol"`
	GoToDefinition string `json:"go_to_definition"`
	FileSwitcher   string `json:"file_switcher"`

	// Панели и интерфейс
	ToggleSidebar  string `json:"toggle_sidebar"`
	ToggleMinimap  string `json:"toggle_minimap"`
	ToggleTerminal string `json:"toggle_terminal"`
	CommandPalette string `json:"command_palette"`
	FileExplorer   string `json:"file_explorer"`

	// Форматирование
	FormatDocument  string `json:"format_document"`
	FormatSelection string `json:"format_selection"`
	CommentLine     string `json:"comment_line"`
	CommentBlock    string `json:"comment_block"`

	// Выделение и курсор
	SelectLine      string `json:"select_line"`
	SelectWord      string `json:"select_word"`
	ExpandSelection string `json:"expand_selection"`
	ShrinkSelection string `json:"shrink_selection"`
	AddCursorAbove  string `json:"add_cursor_above"`
	AddCursorBelow  string `json:"add_cursor_below"`

	// Фолдинг
	FoldBlock   string `json:"fold_block"`
	UnfoldBlock string `json:"unfold_block"`
	FoldAll     string `json:"fold_all"`
	UnfoldAll   string `json:"unfold_all"`

	// Терминал
	OpenTerminal   string `json:"open_terminal"`
	OpenPowerShell string `json:"open_powershell"`
	OpenCMD        string `json:"open_cmd"`

	// Сравнение файлов
	CompareFiles string `json:"compare_files"`

	// Кастомные клавиши
	CustomBindings map[string]string `json:"custom_bindings"`

	// Настройки системы клавиш
	EnableVimBindings    bool `json:"enable_vim_bindings"`
	EnableEmacsBindings  bool `json:"enable_emacs_bindings"`
	EnableVSCodeBindings bool `json:"enable_vscode_bindings"`
}

// ExternalToolsConfig - настройки внешних инструментов
type ExternalToolsConfig struct {
	// Терминалы
	DefaultTerminal  string `json:"default_terminal"` // "cmd", "powershell", "wsl"
	TerminalPath     string `json:"terminal_path"`
	TerminalArgs     string `json:"terminal_args"`
	WorkingDirectory string `json:"working_directory"`

	// Пользовательские инструменты
	CustomTools []CustomTool `json:"custom_tools"`

	// Компиляторы и линтеры
	Linters    LinterConfig    `json:"linters"`
	Formatters FormatterConfig `json:"formatters"`

	// Интеграция с браузером
	DefaultBrowser string `json:"default_browser"`
	BrowserPath    string `json:"browser_path"`

	// Git интеграция
	GitPath       string `json:"git_path"`
	GitAutoFetch  bool   `json:"git_auto_fetch"`
	GitShowBranch bool   `json:"git_show_branch"`
}

// CustomTool - пользовательский инструмент
type CustomTool struct {
	Name             string            `json:"name"`
	DisplayName      string            `json:"display_name"`
	Path             string            `json:"path"`
	Args             string            `json:"args"`
	WorkingDirectory string            `json:"working_directory"`
	ShowOutput       bool              `json:"show_output"`
	RunInTerminal    bool              `json:"run_in_terminal"`
	FileExtensions   []string          `json:"file_extensions"`
	Environment      map[string]string `json:"environment"`
	Enabled          bool              `json:"enabled"`
}

// LinterConfig - настройки линтеров
type LinterConfig struct {
	EnableLinting   bool                      `json:"enable_linting"`
	LintOnSave      bool                      `json:"lint_on_save"`
	LintOnType      bool                      `json:"lint_on_type"`
	ShowErrors      bool                      `json:"show_errors"`
	ShowWarnings    bool                      `json:"show_warnings"`
	LanguageLinters map[string]LanguageLinter `json:"language_linters"`
}

// LanguageLinter - настройки линтера для языка
type LanguageLinter struct {
	Name             string   `json:"name"`
	Path             string   `json:"path"`
	Args             []string `json:"args"`
	Enabled          bool     `json:"enabled"`
	ErrorPattern     string   `json:"error_pattern"`
	WorkingDirectory string   `json:"working_directory"`
}

// FormatterConfig - настройки форматтеров
type FormatterConfig struct {
	EnableFormatting   bool                         `json:"enable_formatting"`
	FormatOnSave       bool                         `json:"format_on_save"`
	LanguageFormatters map[string]LanguageFormatter `json:"language_formatters"`
}

// LanguageFormatter - настройки форматтера для языка
type LanguageFormatter struct {
	Name    string   `json:"name"`
	Path    string   `json:"path"`
	Args    []string `json:"args"`
	Enabled bool     `json:"enabled"`
}

// IntegrationConfig - настройки интеграции
type IntegrationConfig struct {
	// URL для документации
	DocumentationURLs map[string]string `json:"documentation_urls"`

	// GitHub интеграция
	GitHubIntegration bool   `json:"github_integration"`
	GitHubToken       string `json:"github_token"`

	// Language Server Protocol
	LSPServers map[string]LSPServer `json:"lsp_servers"`

	// Плагины и расширения
	PluginDirectory   string `json:"plugin_directory"`
	EnablePlugins     bool   `json:"enable_plugins"`
	AutoUpdatePlugins bool   `json:"auto_update_plugins"`
}

// LSPServer - настройки Language Server
type LSPServer struct {
	Language              string                 `json:"language"`
	Command               string                 `json:"command"`
	Args                  []string               `json:"args"`
	Enabled               bool                   `json:"enabled"`
	InitializationOptions map[string]interface{} `json:"initialization_options"`
}

// AdvancedConfig - расширенные настройки
type AdvancedConfig struct {
	// Производительность
	MaxMemoryUsage  int64   `json:"max_memory_usage"` // в байтах
	MaxCPUUsage     float64 `json:"max_cpu_usage"`    // в процентах
	EnableMulticore bool    `json:"enable_multicore"`
	WorkerThreads   int     `json:"worker_threads"`

	// Логирование и отладка
	EnableLogging bool   `json:"enable_logging"`
	LogLevel      string `json:"log_level"` // "debug", "info", "warn", "error"
	LogFile       string `json:"log_file"`
	LogMaxSize    int64  `json:"log_max_size"`

	// Кэширование
	EnableCaching  bool   `json:"enable_caching"`
	CacheDirectory string `json:"cache_directory"`
	CacheMaxSize   int64  `json:"cache_max_size"`
	CacheTTL       int    `json:"cache_ttl"` // в секундах

	// Сетевые настройки
	ProxyURL  string `json:"proxy_url"`
	Timeout   int    `json:"timeout"` // в секундах
	UserAgent string `json:"user_agent"`

	// Безопасность
	EnableFileEncryption bool   `json:"enable_file_encryption"`
	EncryptionKey        string `json:"encryption_key"`
	SecureMode           bool   `json:"secure_mode"`

	// Экспериментальные функции
	ExperimentalFeatures map[string]bool `json:"experimental_features"`
}

// ConfigManager - менеджер настроек
type ConfigManager struct {
	config          *Config
	configPath      string
	mutex           sync.RWMutex
	watcher         *fsnotify.Watcher
	watcherCancel   context.CancelFunc
	changeCallbacks []func(*Config)
	isLoaded        bool

	saveCh chan saveRequest
	saveWg sync.WaitGroup
}

type saveRequest struct {
	errCh chan error
}

// Валидация настроек
var (
	validThemes          = []string{"dark", "light", "auto"}
	validSortBy          = []string{"name", "type", "size", "date"}
	validPositions       = []string{"left", "right"}
	validStartupBehavior = []string{"blank", "last_session", "welcome"}
	validLogLevels       = []string{"debug", "info", "warn", "error"}

	keyBindingPattern = regexp.MustCompile(`^(Ctrl|Alt|Shift|\+|[A-Za-z0-9])+$`)
)

// DefaultConfig возвращает конфигурацию по умолчанию
func DefaultConfig() *Config {
	return &Config{
		Version:      "1.0.0",
		LastModified: time.Now(),

		App: AppConfig{
			Theme:           "dark",
			Language:        "en",
			WindowWidth:     1400,
			WindowHeight:    900,
			WindowX:         100,
			WindowY:         100,
			WindowMaximized: false,
			LastOpenedFiles: []string{},
			RecentProjects:  []string{},
			StartupBehavior: "last_session",
			CheckUpdates:    true,
			CornerRadius:    6,
		},

		Editor: EditorConfig{
			FontFamily: "JetBrains Mono",
			FontSize:   14,
			LineHeight: 1.2,
			TabSize:    4,
			IndentSize: 4,
			UseSpaces:  true,

			ShowLineNumbers:     true,
			ShowRelativeNumbers: false,
			ShowRuler:           false,
			RulerColumn:         80,
			WordWrap:            false,
			WrapColumn:          120,

			IndentGuides:         true,
			ShowWhitespace:       false,
			ShowEndOfLine:        false,
			HighlightCurrentLine: true,

			AutoIndent:        true,
			SmartIndent:       true,
			AutoCloseBrackets: true,
			AutoCloseQuotes:   true,
			AutoSurround:      true,

			SyntaxHighlighting:    true,
			BracketMatching:       true,
			HighlightMatches:      true,
			HighlightCurrentWord:  true,
			WordHighlightDuration: 2,
			VariableHighlight:     true,

			AutoSave:        true,
			AutoSaveDelay:   300, // 5 минут
			BackupFiles:     true,
			BackupDirectory: "",

			FormatOnSave:       false,
			TrimWhitespace:     true,
			InsertFinalNewline: true,

			CodeFolding:  true,
			FoldComments: false,
			FoldImports:  false,

			SearchCaseSensitive: false,
			SearchWholeWord:     false,
			SearchRegex:         false,
			SearchWrap:          true,

			VimMode:     false,
			VimShowMode: true,

			TrackingKeywords:   []string{"TODO", "FIXME", "NOTE", "HACK", "BUG"},
			MaxFileSize:        104857600, // 100MB
			SupportedEncodings: []string{"UTF-8", "UTF-16", "ASCII", "Windows-1252"},
		},

		Sidebar: SidebarConfig{
			IsVisible: true,
			Width:     280,
			Position:  "left",

			ShowHiddenFiles: false,
			ShowGitIgnored:  true,
			ShowSystemFiles: false,

			SortBy:        "name",
			SortAscending: true,
			DefaultFilter: "All",

			ExpandDirectories: false,
			ShowFileIcons:     true,
			ShowFileSize:      true,
			ShowModifiedDate:  false,

			EnableContextMenu:    true,
			ShowInExplorer:       true,
			EnableFileOperations: true,

			EnableFileWatcher:   true,
			WatchSubdirectories: true,
			WatcherDelay:        500,

			MaxFilesToShow: 1000,
			LazyLoading:    true,
			CacheDirectory: true,
		},

		Minimap: MinimapConfig{
			IsVisible: true,
			Width:     120,
			Position:  "right",

			ShowSyntax:       true,
			ShowLineNumbers:  false,
			ShowIndentGuides: false,
			ShowScrollbar:    false,

			LineHeight:      4.0,
			CharWidth:       2.0,
			FontSize:        3.0,
			MaxCharsPerLine: 100,

			SmoothScrolling: true,
			ClickToScroll:   true,
			AutoHide:        false,
			AutoHideDelay:   3,

			ShowViewport:    true,
			ViewportOpacity: 0.3,
			ViewportColor:   "#0078D4",

			RenderFPS:     60,
			EnableCaching: true,
			MaxCacheSize:  100,
		},

		KeyBindings: KeyBindingsConfig{
			// Файловые операции
			NewFile:    "Ctrl+N",
			OpenFile:   "Ctrl+O",
			SaveFile:   "Ctrl+S",
			SaveAsFile: "Ctrl+Shift+S",
			CloseFile:  "Ctrl+W",
			CloseAll:   "Ctrl+Shift+W",

			// Редактирование
			Cut:       "Ctrl+X",
			Copy:      "Ctrl+C",
			Paste:     "Ctrl+V",
			SelectAll: "Ctrl+A",
			Undo:      "Ctrl+Z",
			Redo:      "Ctrl+Y",

			// Поиск и навигация
			Find:           "Ctrl+F",
			FindNext:       "F3",
			FindPrevious:   "Shift+F3",
			Replace:        "Ctrl+H",
			FindInFiles:    "Ctrl+Shift+F",
			GoToLine:       "Ctrl+G",
			GoToSymbol:     "Ctrl+Shift+O",
			GoToDefinition: "F12",
			FileSwitcher:   "Ctrl+P",

			// Панели и интерфейс
			ToggleSidebar:  "Ctrl+B",
			ToggleMinimap:  "Ctrl+M",
			ToggleTerminal: "Ctrl+`",
			CommandPalette: "Ctrl+Shift+P",
			FileExplorer:   "Ctrl+Shift+E",

			// Форматирование
			FormatDocument:  "Shift+Alt+F",
			FormatSelection: "Ctrl+K Ctrl+F",
			CommentLine:     "Ctrl+/",
			CommentBlock:    "Shift+Alt+A",

			// Выделение и курсор
			SelectLine:      "Ctrl+L",
			SelectWord:      "Ctrl+D",
			ExpandSelection: "Shift+Alt+Right",
			ShrinkSelection: "Shift+Alt+Left",
			AddCursorAbove:  "Ctrl+Alt+Up",
			AddCursorBelow:  "Ctrl+Alt+Down",

			// Фолдинг
			FoldBlock:   "Ctrl+Shift+[",
			UnfoldBlock: "Ctrl+Shift+]",
			FoldAll:     "Ctrl+K Ctrl+0",
			UnfoldAll:   "Ctrl+K Ctrl+J",

			// Терминал
			OpenTerminal:   "Ctrl+Shift+`",
			OpenPowerShell: "Ctrl+Shift+P",
			OpenCMD:        "Ctrl+Shift+C",

			// Сравнение файлов
			CompareFiles: "Ctrl+Shift+D",

			CustomBindings: map[string]string{},

			EnableVimBindings:    false,
			EnableEmacsBindings:  false,
			EnableVSCodeBindings: true,
		},

		ExternalTools: ExternalToolsConfig{
			DefaultTerminal:  "powershell",
			TerminalPath:     "",
			TerminalArgs:     "",
			WorkingDirectory: "",

			CustomTools: []CustomTool{},

			Linters: LinterConfig{
				EnableLinting: true,
				LintOnSave:    true,
				LintOnType:    false,
				ShowErrors:    true,
				ShowWarnings:  true,
				LanguageLinters: map[string]LanguageLinter{
					"go": {
						Name:             "golint",
						Path:             "golint",
						Args:             []string{},
						Enabled:          true,
						ErrorPattern:     `^(.+):(\d+):(\d+):\s*(.+)$`,
						WorkingDirectory: "",
					},
					"python": {
						Name:             "flake8",
						Path:             "flake8",
						Args:             []string{"--max-line-length=120"},
						Enabled:          true,
						ErrorPattern:     `^(.+):(\d+):(\d+):\s*(.+)$`,
						WorkingDirectory: "",
					},
					"rust": {
						Name:             "clippy",
						Path:             "clippy-driver",
						Args:             []string{},
						Enabled:          true,
						ErrorPattern:     `error(?:\[(\w+)\])?: (.+)\s+--> ([^:]+):(\d+):(\d+)`,
						WorkingDirectory: "",
					},
					"c": {
						Name:             "clang-tidy",
						Path:             "clang-tidy",
						Args:             []string{},
						Enabled:          true,
						ErrorPattern:     `([^:]+):(\d+):(\d+):\s*(error|warning):\s*(.+)`,
						WorkingDirectory: "",
					},
					"java": {
						Name:             "checkstyle",
						Path:             "checkstyle",
						Args:             []string{"-c", "/google_checks.xml"},
						Enabled:          true,
						ErrorPattern:     `([^:]+):(\d+):\s*(error|warning):\s*(.+)`,
						WorkingDirectory: "",
					},
				},
			},

			Formatters: FormatterConfig{
				EnableFormatting: true,
				FormatOnSave:     false,
				LanguageFormatters: map[string]LanguageFormatter{
					"go": {
						Name:    "gofmt",
						Path:    "gofmt",
						Args:    []string{},
						Enabled: true,
					},
					"python": {
						Name:    "black",
						Path:    "black",
						Args:    []string{"--line-length=120"},
						Enabled: true,
					},
					"rust": {
						Name:    "rustfmt",
						Path:    "rustfmt",
						Args:    []string{"--emit=stdout"},
						Enabled: true,
					},
					"c": {
						Name:    "clang-format",
						Path:    "clang-format",
						Args:    []string{"-style=LLVM"},
						Enabled: true,
					},
					"java": {
						Name:    "google-java-format",
						Path:    "google-java-format",
						Args:    []string{"-"},
						Enabled: true,
					},
				},
			},

			DefaultBrowser: "",
			BrowserPath:    "",

			GitPath:       "git",
			GitAutoFetch:  false,
			GitShowBranch: true,
		},

		Integration: IntegrationConfig{
			DocumentationURLs: map[string]string{
				"go":     "https://pkg.go.dev/",
				"python": "https://docs.python.org/3/library/",
				"rust":   "https://doc.rust-lang.org/std/",
				"java":   "https://docs.oracle.com/javase/",
				"c":      "https://en.cppreference.com/w/c/",
			},

			GitHubIntegration: false,
			GitHubToken:       "",

			LSPServers: map[string]LSPServer{
				"go": {
					Language: "go",
					Command:  "gopls",
					Args:     []string{},
					Enabled:  false,
				},
			},

			PluginDirectory:   "",
			EnablePlugins:     false,
			AutoUpdatePlugins: false,
		},

		Advanced: AdvancedConfig{
			MaxMemoryUsage:  1073741824, // 1GB
			MaxCPUUsage:     80.0,
			EnableMulticore: true,
			WorkerThreads:   4,

			EnableLogging: false,
			LogLevel:      "info",
			LogFile:       "",
			LogMaxSize:    10485760, // 10MB

			EnableCaching:  true,
			CacheDirectory: "",
			CacheMaxSize:   104857600, // 100MB
			CacheTTL:       3600,      // 1 час

			ProxyURL:  "",
			Timeout:   30,
			UserAgent: "Programmer's Notepad/1.0",

			EnableFileEncryption: false,
			EncryptionKey:        "",
			SecureMode:           false,

			ExperimentalFeatures: map[string]bool{
				"ai_completion":      false,
				"collaborative_edit": false,
				"cloud_sync":         false,
			},
		},
	}
}

// NewConfigManager создает новый менеджер настроек
func NewConfigManager(configPath string) *ConfigManager {
	if configPath == "" {
		// Определяем путь к файлу конфигурации
		configDir := getConfigDirectory()
		configPath = filepath.Join(configDir, "config.json")
	}

	manager := &ConfigManager{
		configPath:      configPath,
		changeCallbacks: []func(*Config){},
	}
	manager.startSaveWorker()

	return manager
}

func (cm *ConfigManager) startSaveWorker() {
	cm.saveCh = make(chan saveRequest, 1)
	cm.saveWg.Add(1)
	go func() {
		defer cm.saveWg.Done()
		for req := range cm.saveCh {
			cm.mutex.Lock()
			err := cm.saveConfigUnsafe()
			cm.mutex.Unlock()
			if req.errCh != nil {
				req.errCh <- err
				close(req.errCh)
			} else if err != nil {
				log.Printf("Error saving config: %v", err)
			}
		}
	}()
}

// LoadConfig загружает настройки из файла
func (cm *ConfigManager) LoadConfig() (*Config, error) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	// Если файл не существует, создаем настройки по умолчанию
	if _, err := os.Stat(cm.configPath); os.IsNotExist(err) {
		cm.config = DefaultConfig()
		cm.config.ConfigPath = cm.configPath

		// Создаем директорию если не существует
		dir := filepath.Dir(cm.configPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("cannot create config directory: %v", err)
		}

		// Сохраняем конфигурацию по умолчанию
		if err := cm.saveConfigUnsafe(); err != nil {
			return nil, fmt.Errorf("cannot save default config: %v", err)
		}

		cm.isLoaded = true
		return cm.config, nil
	}

	// Читаем файл конфигурации
	data, err := ioutil.ReadFile(cm.configPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read config file: %v", err)
	}

	// Парсим JSON
	config := DefaultConfig() // Начинаем с default значений
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("cannot parse config file: %v", err)
	}

	config.ConfigPath = cm.configPath

	// Валидируем настройки
	if err := cm.validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid config: %v", err)
	}

	// Применяем миграции если версия изменилась
	if err := cm.migrateConfig(config); err != nil {
		return nil, fmt.Errorf("config migration failed: %v", err)
	}

	cm.config = config
	cm.isLoaded = true

	// Запускаем file watcher
	cm.startWatching()

	return cm.config, nil
}

// SaveConfig сохраняет настройки синхронно через воркер
func (cm *ConfigManager) SaveConfig() error {
	if cm.saveCh == nil {
		return fmt.Errorf("save worker not started")
	}
	errCh := make(chan error, 1)
	cm.saveCh <- saveRequest{errCh: errCh}
	return <-errCh
}

// SaveConfigAsync ставит запрос на сохранение в очередь
func (cm *ConfigManager) SaveConfigAsync() {
	if cm.saveCh == nil {
		return
	}
	select {
	case cm.saveCh <- saveRequest{}:
	default:
		// если запрос уже в очереди, не блокируемся
	}
}

// saveConfigUnsafe внутренний метод сохранения (без блокировки)
func (cm *ConfigManager) saveConfigUnsafe() error {
	if cm.config == nil {
		return fmt.Errorf("config is not loaded")
	}

	// Обновляем метаданные
	cm.config.LastModified = time.Now()

	// Валидируем перед сохранением
	if err := cm.validateConfig(cm.config); err != nil {
		return fmt.Errorf("invalid config: %v", err)
	}

	// Сериализуем в JSON с красивым форматированием
	data, err := json.MarshalIndent(cm.config, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot serialize config: %v", err)
	}

	// Создаем backup старого файла
	if err := cm.createBackup(); err != nil {
		// Логируем ошибку, но не прерываем сохранение
		log.Printf("Warning: cannot create config backup: %v", err)
	}

	// Записываем файл
	if err := ioutil.WriteFile(cm.configPath, data, 0644); err != nil {
		return fmt.Errorf("cannot write config file: %v", err)
	}

	return nil
}

// GetConfig возвращает текущую конфигурацию
func (cm *ConfigManager) GetConfig() *Config {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	if cm.config == nil {
		return DefaultConfig()
	}

	// Возвращаем копию для безопасности
	return cm.copyConfig(cm.config)
}

// UpdateConfig обновляет конфигурацию
func (cm *ConfigManager) UpdateConfig(updater func(*Config)) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	if cm.config == nil {
		cm.config = DefaultConfig()
	}

	// Применяем изменения
	updater(cm.config)

	// Сохраняем
	if err := cm.saveConfigUnsafe(); err != nil {
		return err
	}

	// Уведомляем о изменениях
	cm.notifyCallbacks()

	return nil
}

// GetString возвращает строковое значение настройки
func (cm *ConfigManager) GetString(path string) string {
	value := cm.getValue(path)
	if str, ok := value.(string); ok {
		return str
	}
	return ""
}

// GetInt возвращает целое значение настройки
func (cm *ConfigManager) GetInt(path string) int {
	value := cm.getValue(path)
	if i, ok := value.(int); ok {
		return i
	}
	if f, ok := value.(float64); ok {
		return int(f)
	}
	return 0
}

// GetFloat возвращает дробное значение настройки
func (cm *ConfigManager) GetFloat(path string) float32 {
	value := cm.getValue(path)
	if f, ok := value.(float32); ok {
		return f
	}
	if f, ok := value.(float64); ok {
		return float32(f)
	}
	return 0.0
}

// GetBool возвращает булево значение настройки
func (cm *ConfigManager) GetBool(path string) bool {
	value := cm.getValue(path)
	if b, ok := value.(bool); ok {
		return b
	}
	return false
}

// SetValue устанавливает значение настройки
func (cm *ConfigManager) SetValue(path string, value interface{}) error {
	return cm.UpdateConfig(func(config *Config) {
		cm.setValue(config, path, value)
	})
}

// getValue получает значение по пути (например "editor.font_size")
func (cm *ConfigManager) getValue(path string) interface{} {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	if cm.config == nil {
		return nil
	}

	return cm.getValueFromConfig(cm.config, path)
}

// getValueFromConfig извлекает значение из конфигурации
func (cm *ConfigManager) getValueFromConfig(config *Config, path string) interface{} {
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return nil
	}

	section := parts[0]
	key := parts[1]

	switch section {
	case "app":
		return cm.getAppValue(&config.App, key)
	case "editor":
		return cm.getEditorValue(&config.Editor, key)
	case "sidebar":
		return cm.getSidebarValue(&config.Sidebar, key)
	case "minimap":
		return cm.getMinimapValue(&config.Minimap, key)
	case "key_bindings":
		return cm.getKeyBindingsValue(&config.KeyBindings, key)
	case "external_tools":
		return cm.getExternalToolsValue(&config.ExternalTools, key)
	case "integration":
		return cm.getIntegrationValue(&config.Integration, key)
	case "advanced":
		return cm.getAdvancedValue(&config.Advanced, key)
	}

	return nil
}

// setValue устанавливает значение в конфигурации
func (cm *ConfigManager) setValue(config *Config, path string, value interface{}) {
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return
	}

	section := parts[0]
	key := parts[1]

	switch section {
	case "app":
		cm.setAppValue(&config.App, key, value)
	case "editor":
		cm.setEditorValue(&config.Editor, key, value)
	case "sidebar":
		cm.setSidebarValue(&config.Sidebar, key, value)
	case "minimap":
		cm.setMinimapValue(&config.Minimap, key, value)
	case "key_bindings":
		cm.setKeyBindingsValue(&config.KeyBindings, key, value)
	case "external_tools":
		cm.setExternalToolsValue(&config.ExternalTools, key, value)
	case "integration":
		cm.setIntegrationValue(&config.Integration, key, value)
	case "advanced":
		cm.setAdvancedValue(&config.Advanced, key, value)
	}
}

// Методы для работы с разными секциями конфигурации

// getAppValue извлекает значение из секции App
func (cm *ConfigManager) getAppValue(app *AppConfig, key string) interface{} {
	switch key {
	case "theme":
		return app.Theme
	case "language":
		return app.Language
	case "window_width":
		return app.WindowWidth
	case "window_height":
		return app.WindowHeight
	case "window_maximized":
		return app.WindowMaximized
	case "startup_behavior":
		return app.StartupBehavior
	case "check_updates":
		return app.CheckUpdates
	}
	return nil
}

// setAppValue устанавливает значение в секции App
func (cm *ConfigManager) setAppValue(app *AppConfig, key string, value interface{}) {
	switch key {
	case "theme":
		if str, ok := value.(string); ok {
			app.Theme = str
		}
	case "language":
		if str, ok := value.(string); ok {
			app.Language = str
		}
	case "window_width":
		if i, ok := value.(int); ok {
			app.WindowWidth = i
		}
	case "window_height":
		if i, ok := value.(int); ok {
			app.WindowHeight = i
		}
	case "window_maximized":
		if b, ok := value.(bool); ok {
			app.WindowMaximized = b
		}
	case "startup_behavior":
		if str, ok := value.(string); ok {
			app.StartupBehavior = str
		}
	case "check_updates":
		if b, ok := value.(bool); ok {
			app.CheckUpdates = b
		}
	}
}

// getEditorValue извлекает значение из секции Editor
func (cm *ConfigManager) getEditorValue(editor *EditorConfig, key string) interface{} {
	switch key {
	case "font_family":
		return editor.FontFamily
	case "font_size":
		return editor.FontSize
	case "tab_size":
		return editor.TabSize
	case "show_line_numbers":
		return editor.ShowLineNumbers
	case "word_wrap":
		return editor.WordWrap
	case "indent_guides":
		return editor.IndentGuides
	case "auto_save":
		return editor.AutoSave
	case "auto_save_delay":
		return editor.AutoSaveDelay
	case "syntax_highlighting":
		return editor.SyntaxHighlighting
	case "bracket_matching":
		return editor.BracketMatching
	case "highlight_current_word":
		return editor.HighlightCurrentWord
	case "word_highlight_duration":
		return editor.WordHighlightDuration
	case "vim_mode":
		return editor.VimMode
	}
	return nil
}

// setEditorValue устанавливает значение в секции Editor
func (cm *ConfigManager) setEditorValue(editor *EditorConfig, key string, value interface{}) {
	switch key {
	case "font_family":
		if str, ok := value.(string); ok {
			editor.FontFamily = str
		}
	case "font_size":
		if f, ok := value.(float32); ok {
			editor.FontSize = f
		} else if f, ok := value.(float64); ok {
			editor.FontSize = float32(f)
		}
	case "tab_size":
		if i, ok := value.(int); ok {
			editor.TabSize = i
		}
	case "show_line_numbers":
		if b, ok := value.(bool); ok {
			editor.ShowLineNumbers = b
		}
	case "word_wrap":
		if b, ok := value.(bool); ok {
			editor.WordWrap = b
		}
	case "indent_guides":
		if b, ok := value.(bool); ok {
			editor.IndentGuides = b
		}
	case "auto_save":
		if b, ok := value.(bool); ok {
			editor.AutoSave = b
		}
	case "auto_save_delay":
		if i, ok := value.(int); ok {
			editor.AutoSaveDelay = i
		}
	case "syntax_highlighting":
		if b, ok := value.(bool); ok {
			editor.SyntaxHighlighting = b
		}
	case "bracket_matching":
		if b, ok := value.(bool); ok {
			editor.BracketMatching = b
		}
	case "highlight_current_word":
		if b, ok := value.(bool); ok {
			editor.HighlightCurrentWord = b
		}
	case "word_highlight_duration":
		if i, ok := value.(int); ok {
			editor.WordHighlightDuration = i
		}
	case "vim_mode":
		if b, ok := value.(bool); ok {
			editor.VimMode = b
		}
	}
}

// getSidebarValue извлекает значение из секции Sidebar
func (cm *ConfigManager) getSidebarValue(sidebar *SidebarConfig, key string) interface{} {
	switch key {
	case "is_visible":
		return sidebar.IsVisible
	case "width":
		return sidebar.Width
	case "show_hidden_files":
		return sidebar.ShowHiddenFiles
	case "sort_by":
		return sidebar.SortBy
	case "sort_ascending":
		return sidebar.SortAscending
	case "default_filter":
		return sidebar.DefaultFilter
	case "enable_file_watcher":
		return sidebar.EnableFileWatcher
	}
	return nil
}

// setSidebarValue устанавливает значение в секции Sidebar
func (cm *ConfigManager) setSidebarValue(sidebar *SidebarConfig, key string, value interface{}) {
	switch key {
	case "is_visible":
		if b, ok := value.(bool); ok {
			sidebar.IsVisible = b
		}
	case "width":
		if f, ok := value.(float32); ok {
			sidebar.Width = f
		}
	case "show_hidden_files":
		if b, ok := value.(bool); ok {
			sidebar.ShowHiddenFiles = b
		}
	case "sort_by":
		if str, ok := value.(string); ok {
			sidebar.SortBy = str
		}
	case "sort_ascending":
		if b, ok := value.(bool); ok {
			sidebar.SortAscending = b
		}
	case "default_filter":
		if str, ok := value.(string); ok {
			sidebar.DefaultFilter = str
		}
	case "enable_file_watcher":
		if b, ok := value.(bool); ok {
			sidebar.EnableFileWatcher = b
		}
	}
}

// getMinimapValue извлекает значение из секции Minimap
func (cm *ConfigManager) getMinimapValue(minimap *MinimapConfig, key string) interface{} {
	switch key {
	case "is_visible":
		return minimap.IsVisible
	case "width":
		return minimap.Width
	case "show_syntax":
		return minimap.ShowSyntax
	case "show_line_numbers":
		return minimap.ShowLineNumbers
	case "smooth_scrolling":
		return minimap.SmoothScrolling
	case "auto_hide":
		return minimap.AutoHide
	}
	return nil
}

// setMinimapValue устанавливает значение в секции Minimap
func (cm *ConfigManager) setMinimapValue(minimap *MinimapConfig, key string, value interface{}) {
	switch key {
	case "is_visible":
		if b, ok := value.(bool); ok {
			minimap.IsVisible = b
		}
	case "width":
		if f, ok := value.(float32); ok {
			minimap.Width = f
		}
	case "show_syntax":
		if b, ok := value.(bool); ok {
			minimap.ShowSyntax = b
		}
	case "show_line_numbers":
		if b, ok := value.(bool); ok {
			minimap.ShowLineNumbers = b
		}
	case "smooth_scrolling":
		if b, ok := value.(bool); ok {
			minimap.SmoothScrolling = b
		}
	case "auto_hide":
		if b, ok := value.(bool); ok {
			minimap.AutoHide = b
		}
	}
}

// getKeyBindingsValue извлекает значение из секции KeyBindings
func (cm *ConfigManager) getKeyBindingsValue(kb *KeyBindingsConfig, key string) interface{} {
	switch key {
	case "new_file":
		return kb.NewFile
	case "open_file":
		return kb.OpenFile
	case "save_file":
		return kb.SaveFile
	case "find":
		return kb.Find
	case "replace":
		return kb.Replace
	case "go_to_line":
		return kb.GoToLine
	case "file_switcher":
		return kb.FileSwitcher
	case "toggle_sidebar":
		return kb.ToggleSidebar
	case "toggle_minimap":
		return kb.ToggleMinimap
	case "command_palette":
		return kb.CommandPalette
	case "compare_files":
		return kb.CompareFiles
	case "enable_vim_bindings":
		return kb.EnableVimBindings
	}
	return nil
}

// setKeyBindingsValue устанавливает значение в секции KeyBindings
func (cm *ConfigManager) setKeyBindingsValue(kb *KeyBindingsConfig, key string, value interface{}) {
	switch key {
	case "new_file":
		if str, ok := value.(string); ok {
			kb.NewFile = str
		}
	case "open_file":
		if str, ok := value.(string); ok {
			kb.OpenFile = str
		}
	case "save_file":
		if str, ok := value.(string); ok {
			kb.SaveFile = str
		}
	case "find":
		if str, ok := value.(string); ok {
			kb.Find = str
		}
	case "replace":
		if str, ok := value.(string); ok {
			kb.Replace = str
		}
	case "go_to_line":
		if str, ok := value.(string); ok {
			kb.GoToLine = str
		}
	case "file_switcher":
		if str, ok := value.(string); ok {
			kb.FileSwitcher = str
		}
	case "toggle_sidebar":
		if str, ok := value.(string); ok {
			kb.ToggleSidebar = str
		}
	case "toggle_minimap":
		if str, ok := value.(string); ok {
			kb.ToggleMinimap = str
		}
	case "command_palette":
		if str, ok := value.(string); ok {
			kb.CommandPalette = str
		}
	case "compare_files":
		if str, ok := value.(string); ok {
			kb.CompareFiles = str
		}
	case "enable_vim_bindings":
		if b, ok := value.(bool); ok {
			kb.EnableVimBindings = b
		}
	}
}

// getExternalToolsValue извлекает значение из секции ExternalTools
func (cm *ConfigManager) getExternalToolsValue(et *ExternalToolsConfig, key string) interface{} {
	switch key {
	case "default_terminal":
		return et.DefaultTerminal
	case "terminal_path":
		return et.TerminalPath
	case "git_path":
		return et.GitPath
	}
	return nil
}

// setExternalToolsValue устанавливает значение в секции ExternalTools
func (cm *ConfigManager) setExternalToolsValue(et *ExternalToolsConfig, key string, value interface{}) {
	switch key {
	case "default_terminal":
		if str, ok := value.(string); ok {
			et.DefaultTerminal = str
		}
	case "terminal_path":
		if str, ok := value.(string); ok {
			et.TerminalPath = str
		}
	case "git_path":
		if str, ok := value.(string); ok {
			et.GitPath = str
		}
	}
}

// getIntegrationValue извлекает значение из секции Integration
func (cm *ConfigManager) getIntegrationValue(i *IntegrationConfig, key string) interface{} {
	switch key {
	case "github_integration":
		return i.GitHubIntegration
	case "enable_plugins":
		return i.EnablePlugins
	}
	return nil
}

// setIntegrationValue устанавливает значение в секции Integration
func (cm *ConfigManager) setIntegrationValue(i *IntegrationConfig, key string, value interface{}) {
	switch key {
	case "github_integration":
		if b, ok := value.(bool); ok {
			i.GitHubIntegration = b
		}
	case "enable_plugins":
		if b, ok := value.(bool); ok {
			i.EnablePlugins = b
		}
	}
}

// getAdvancedValue извлекает значение из секции Advanced
func (cm *ConfigManager) getAdvancedValue(a *AdvancedConfig, key string) interface{} {
	switch key {
	case "enable_logging":
		return a.EnableLogging
	case "log_level":
		return a.LogLevel
	case "enable_caching":
		return a.EnableCaching
	case "secure_mode":
		return a.SecureMode
	}
	return nil
}

// setAdvancedValue устанавливает значение в секции Advanced
func (cm *ConfigManager) setAdvancedValue(a *AdvancedConfig, key string, value interface{}) {
	switch key {
	case "enable_logging":
		if b, ok := value.(bool); ok {
			a.EnableLogging = b
		}
	case "log_level":
		if str, ok := value.(string); ok {
			a.LogLevel = str
		}
	case "enable_caching":
		if b, ok := value.(bool); ok {
			a.EnableCaching = b
		}
	case "secure_mode":
		if b, ok := value.(bool); ok {
			a.SecureMode = b
		}
	}
}

// Валидация настроек

// validateConfig проверяет корректность настроек
func (cm *ConfigManager) validateConfig(config *Config) error {
	if config == nil {
		return fmt.Errorf("config is nil")
	}

	// Валидируем App секцию
	if err := cm.validateAppConfig(&config.App); err != nil {
		return fmt.Errorf("app config: %v", err)
	}

	// Валидируем Editor секцию
	if err := cm.validateEditorConfig(&config.Editor); err != nil {
		return fmt.Errorf("editor config: %v", err)
	}

	// Валидируем Sidebar секцию
	if err := cm.validateSidebarConfig(&config.Sidebar); err != nil {
		return fmt.Errorf("sidebar config: %v", err)
	}

	// Валидируем Minimap секцию
	if err := cm.validateMinimapConfig(&config.Minimap); err != nil {
		return fmt.Errorf("minimap config: %v", err)
	}

	// Валидируем KeyBindings секцию
	if err := cm.validateKeyBindingsConfig(&config.KeyBindings); err != nil {
		return fmt.Errorf("key bindings config: %v", err)
	}

	return nil
}

// validateAppConfig валидирует настройки приложения
func (cm *ConfigManager) validateAppConfig(app *AppConfig) error {
	if !contains(validThemes, app.Theme) {
		return fmt.Errorf("invalid theme: %s", app.Theme)
	}

	if app.WindowWidth < 400 || app.WindowWidth > 10000 {
		return fmt.Errorf("invalid window width: %d", app.WindowWidth)
	}

	if app.WindowHeight < 300 || app.WindowHeight > 10000 {
		return fmt.Errorf("invalid window height: %d", app.WindowHeight)
	}

	if !contains(validStartupBehavior, app.StartupBehavior) {
		return fmt.Errorf("invalid startup behavior: %s", app.StartupBehavior)
	}

	return nil
}

// validateEditorConfig валидирует настройки редактора
func (cm *ConfigManager) validateEditorConfig(editor *EditorConfig) error {
	if editor.FontSize < 6 || editor.FontSize > 72 {
		return fmt.Errorf("invalid font size: %.1f", editor.FontSize)
	}

	if editor.TabSize < 1 || editor.TabSize > 16 {
		return fmt.Errorf("invalid tab size: %d", editor.TabSize)
	}

	if editor.AutoSaveDelay < 10 || editor.AutoSaveDelay > 3600 {
		return fmt.Errorf("invalid auto save delay: %d", editor.AutoSaveDelay)
	}
	if editor.WordHighlightDuration < 0 || editor.WordHighlightDuration > 60 {
		return fmt.Errorf("invalid word highlight duration: %d", editor.WordHighlightDuration)
	}
	if editor.MaxFileSize < 1024 || editor.MaxFileSize > 1073741824 { // 1KB - 1GB
		return fmt.Errorf("invalid max file size: %d", editor.MaxFileSize)
	}

	return nil
}

// validateSidebarConfig валидирует настройки sidebar
func (cm *ConfigManager) validateSidebarConfig(sidebar *SidebarConfig) error {
	if sidebar.Width < 100 || sidebar.Width > 800 {
		return fmt.Errorf("invalid sidebar width: %.1f", sidebar.Width)
	}

	if !contains(validSortBy, sidebar.SortBy) {
		return fmt.Errorf("invalid sort by: %s", sidebar.SortBy)
	}

	if !contains(validPositions, sidebar.Position) {
		return fmt.Errorf("invalid sidebar position: %s", sidebar.Position)
	}

	if sidebar.MaxFilesToShow < 10 || sidebar.MaxFilesToShow > 10000 {
		return fmt.Errorf("invalid max files to show: %d", sidebar.MaxFilesToShow)
	}

	return nil
}

// validateMinimapConfig валидирует настройки minimap
func (cm *ConfigManager) validateMinimapConfig(minimap *MinimapConfig) error {
	if minimap.Width < 50 || minimap.Width > 400 {
		return fmt.Errorf("invalid minimap width: %.1f", minimap.Width)
	}

	if !contains(validPositions, minimap.Position) {
		return fmt.Errorf("invalid minimap position: %s", minimap.Position)
	}

	if minimap.LineHeight < 0.5 || minimap.LineHeight > 10 {
		return fmt.Errorf("invalid line height: %.1f", minimap.LineHeight)
	}

	if minimap.RenderFPS < 1 || minimap.RenderFPS > 120 {
		return fmt.Errorf("invalid render FPS: %d", minimap.RenderFPS)
	}

	return nil
}

// validateKeyBindingsConfig валидирует настройки горячих клавиш
func (cm *ConfigManager) validateKeyBindingsConfig(kb *KeyBindingsConfig) error {
	keyBindings := map[string]string{
		"new_file":        kb.NewFile,
		"open_file":       kb.OpenFile,
		"save_file":       kb.SaveFile,
		"find":            kb.Find,
		"replace":         kb.Replace,
		"go_to_line":      kb.GoToLine,
		"file_switcher":   kb.FileSwitcher,
		"toggle_sidebar":  kb.ToggleSidebar,
		"toggle_minimap":  kb.ToggleMinimap,
		"command_palette": kb.CommandPalette,
		"compare_files":   kb.CompareFiles,
	}

	for action, binding := range keyBindings {
		if binding == "" {
			continue // Пустые привязки разрешены
		}

		if !keyBindingPattern.MatchString(binding) {
			return fmt.Errorf("invalid key binding for %s: %s", action, binding)
		}
	}

	// Проверяем дубликаты
	used := make(map[string]string)
	for action, binding := range keyBindings {
		if binding == "" {
			continue
		}

		if existingAction, exists := used[binding]; exists {
			return fmt.Errorf("duplicate key binding '%s' for %s and %s", binding, action, existingAction)
		}

		used[binding] = action
	}

	return nil
}

// Миграция конфигурации

// migrateConfig выполняет миграцию настроек при изменении версии
func (cm *ConfigManager) migrateConfig(config *Config) error {
	currentVersion := "1.0.0"

	if config.Version == currentVersion {
		return nil // Миграция не нужна
	}

	// Здесь можно добавить логику миграции для будущих версий
	// Например:
	// if config.Version == "0.9.0" {
	//     // Миграция с версии 0.9.0 на 1.0.0
	//     migrateFrom090To100(config)
	// }

	config.Version = currentVersion

	return nil
}

// File watching

// startWatching запускает наблюдение за файлом конфигурации
func (cm *ConfigManager) startWatching() {
	if cm.watcher != nil {
		cm.stopWatching()
	}

	var err error
	cm.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		log.Printf("Cannot create config file watcher: %v", err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	cm.watcherCancel = cancel

	go cm.watcherWorker(ctx)

	err = cm.watcher.Add(cm.configPath)
	if err != nil {
		log.Printf("Cannot watch config file: %v", err)
		cm.stopWatching()
	}
}

// stopWatching останавливает наблюдение
func (cm *ConfigManager) stopWatching() {
	if cm.watcherCancel != nil {
		cm.watcherCancel()
	}

	if cm.watcher != nil {
		cm.watcher.Close()
		cm.watcher = nil
	}
}

// watcherWorker обрабатывает события файловой системы
func (cm *ConfigManager) watcherWorker(ctx context.Context) {
	for {
		select {
		case event, ok := <-cm.watcher.Events:
			if !ok {
				return
			}

			if event.Op&fsnotify.Write == fsnotify.Write {
				// Конфигурационный файл был изменен
				go cm.reloadConfig()
			}

		case err, ok := <-cm.watcher.Errors:
			if !ok {
				return
			}

			log.Printf("Config file watcher error: %v", err)

		case <-ctx.Done():
			return
		}
	}
}

// reloadConfig перезагружает конфигурацию из файла
func (cm *ConfigManager) reloadConfig() {
	// Небольшая задержка, чтобы файл полностью записался
	time.Sleep(100 * time.Millisecond)

	// Загружаем новую конфигурацию
	newConfig, err := cm.LoadConfig()
	if err != nil {
		log.Printf("Error reloading config: %v", err)
		return
	}

	// Применяем новую конфигурацию
	cm.mutex.Lock()
	cm.config = newConfig
	cm.mutex.Unlock()

	// Уведомляем о изменениях
	cm.notifyCallbacks()

	log.Println("Configuration reloaded successfully")
}

// Callback система

// OnConfigChanged добавляет callback для уведомления об изменениях
func (cm *ConfigManager) OnConfigChanged(callback func(*Config)) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	cm.changeCallbacks = append(cm.changeCallbacks, callback)
}

// notifyCallbacks уведомляет все callbacks об изменениях
func (cm *ConfigManager) notifyCallbacks() {
	for _, callback := range cm.changeCallbacks {
		go callback(cm.config) // Выполняем в отдельной goroutine
	}
}

// Вспомогательные методы

// getConfigDirectory возвращает директорию для файла конфигурации
func getConfigDirectory() string {
	// Попробуем получить пользовательскую директорию конфигурации
	if configDir := os.Getenv("APPDATA"); configDir != "" {
		return filepath.Join(configDir, "ProgrammersNotepad")
	}

	// Fallback на домашнюю директорию
	if homeDir, err := os.UserHomeDir(); err == nil {
		return filepath.Join(homeDir, ".programmers-notepad")
	}

	// Последний fallback на текущую директорию
	return "."
}

// createBackup создает backup файла конфигурации
func (cm *ConfigManager) createBackup() error {
	if _, err := os.Stat(cm.configPath); os.IsNotExist(err) {
		return nil // Файл не существует, backup не нужен
	}

	backupPath := cm.configPath + ".backup"

	// Копируем файл
	input, err := ioutil.ReadFile(cm.configPath)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(backupPath, input, 0644)
	if err != nil {
		return err
	}

	return nil
}

// copyConfig создает глубокую копию конфигурации
func (cm *ConfigManager) copyConfig(src *Config) *Config {
	// Простая реализация через JSON сериализацию
	// В production коде лучше использовать более эффективный способ
	data, err := json.Marshal(src)
	if err != nil {
		return DefaultConfig()
	}

	var dst Config
	if err := json.Unmarshal(data, &dst); err != nil {
		return DefaultConfig()
	}

	return &dst
}

// contains проверяет наличие элемента в слайсе
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// Cleanup очищает ресурсы менеджера настроек
func (cm *ConfigManager) Cleanup() {
	cm.stopWatching()

	if cm.saveCh != nil {
		close(cm.saveCh)
		cm.saveWg.Wait()
	}
}

// Экспортируемые функции для удобства

// LoadConfig загружает конфигурацию из файла по умолчанию
func LoadConfig() (*Config, error) {
	manager := NewConfigManager("")
	return manager.LoadConfig()
}
