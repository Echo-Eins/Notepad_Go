package main

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
)

// HotkeyManager - менеджер горячих клавиш
type HotkeyManager struct {
	// Основные компоненты
	config *Config
	window fyne.Window
	app    *App // Ссылка на основное приложение

	// Зарегистрированные клавиши
	shortcuts        map[string]*RegisteredShortcut
	globalShortcuts  map[string]*GlobalShortcut
	contextShortcuts map[HotkeyContext]map[string]*RegisteredShortcut

	// Режимы ввода
	currentMode InputMode
	vimState    *VimState
	emacsState  *EmacsState

	// Контекст
	currentContext HotkeyContext
	focusedWidget  fyne.Focusable

	// Callbacks для действий
	actions map[string]HotkeyAction

	// Мультиклавишные комбинации
	pendingKeys     []fyne.KeyName
	pendingTimeout  *time.Timer
	pendingCallback func()

	// Синхронизация
	mutex sync.RWMutex

	// Состояние модификаторов
	modifierState ModifierState

	// Конфликты и приоритеты
	conflictResolver *ConflictResolver

	// Отладка и логирование
	debugMode bool
	keyLogger *KeyLogger
}

// RegisteredShortcut - зарегистрированная горячая клавиша
type RegisteredShortcut struct {
	ID            string
	Shortcut      *desktop.CustomShortcut
	Action        HotkeyAction
	Context       HotkeyContext
	Mode          InputMode
	Description   string
	Priority      int
	Enabled       bool
	ConflictsWith []string

	// Условия активации
	RequiredContext HotkeyContext
	RequiredMode    InputMode
	RequiredFocus   string

	// Метаданные
	Category string
	Tags     []string
	LastUsed time.Time
	UseCount int
}

// GlobalShortcut - глобальная горячая клавиша (работает вне приложения)
type GlobalShortcut struct {
	ID             string
	KeyCombination string
	Action         HotkeyAction
	Enabled        bool
	Description    string
}

// HotkeyAction - функция-обработчик горячей клавиши
type HotkeyAction func(context HotkeyContext) bool

// HotkeyContext - контекст выполнения горячей клавиши
type HotkeyContext int

const (
	ContextGlobal HotkeyContext = iota
	ContextEditor
	ContextSidebar
	ContextMinimap
	ContextDialog
	ContextTerminal
	ContextSearch
	ContextMenu
	ContextCommandPalette
)

// InputMode - режим ввода
type InputMode int

const (
	ModeNormal InputMode = iota
	ModeVim
	ModeEmacs
	ModeInsert
	ModeVisual
	ModeCommand
)

// VimState - состояние Vim режима
type VimState struct {
	Mode            VimMode
	PendingKeys     []fyne.KeyName
	Register        string
	Count           int
	LastCommand     string
	YankBuffer      string
	Marks           map[string]TextPosition
	JumpList        []TextPosition
	SearchTerm      string
	SearchDirection SearchDirection

	// Операторы и движения
	PendingOperator string
	PendingMotion   string

	// Режим записи макросов
	RecordingMacro bool
	MacroRegister  string
	MacroKeys      []fyne.KeyName

	// Состояние ex команд
	ExCommandMode   bool
	ExCommandBuffer string
}

// EmacsState - состояние Emacs режима
type EmacsState struct {
	PendingKeys    []fyne.KeyName
	KillRing       []string
	KillRingIndex  int
	Mark           *TextPosition
	MarkActive     bool
	PrefixArgument int
	UniversalArg   bool
	LastCommand    string

	// Режим поиска
	IncrementalSearch bool
	SearchTerm        string
	SearchDirection   SearchDirection
	SearchStartPos    TextPosition
}

// VimMode - режимы Vim
type VimMode int

const (
	VimNormal VimMode = iota
	VimInsert
	VimVisual
	VimVisualLine
	VimVisualBlock
	VimCommand
	VimReplace
)

// SearchDirection - направление поиска
type SearchDirection int

const (
	SearchForward SearchDirection = iota
	SearchBackward
)

// ModifierState - состояние клавиш-модификаторов
type ModifierState struct {
	Ctrl  bool
	Alt   bool
	Shift bool
	Super bool // Windows key
}

// ConflictResolver - резолвер конфликтов горячих клавиш
type ConflictResolver struct {
	rules      []ConflictRule
	priorities map[HotkeyContext]int
	overrides  map[string]string
}

// ConflictRule - правило разрешения конфликтов
type ConflictRule struct {
	Pattern    *regexp.Regexp
	Resolution ConflictResolution
	Priority   int
}

// ConflictResolution - способ разрешения конфликта
type ConflictResolution int

const (
	ResolutionHighestPriority ConflictResolution = iota
	ResolutionContextSpecific
	ResolutionModeSpecific
	ResolutionUserChoice
	ResolutionDisable
)

// KeyLogger - логгер нажатий клавиш для отладки
type KeyLogger struct {
	enabled    bool
	maxEntries int
	entries    []KeyLogEntry
	mutex      sync.Mutex
}

// KeyLogEntry - запись в логе клавиш
type KeyLogEntry struct {
	Timestamp time.Time
	Key       fyne.KeyName
	Modifiers ModifierState
	Context   HotkeyContext
	Mode      InputMode
	Action    string
	Result    string
}

func (hm *HotkeyManager) SetApp(app *App) {
	hm.app = app
}

// NewHotkeyManager создает новый менеджер горячих клавиш
func NewHotkeyManager(config *Config, window fyne.Window) *HotkeyManager {
	hm := &HotkeyManager{
		config:           config,
		window:           window,
		shortcuts:        make(map[string]*RegisteredShortcut),
		globalShortcuts:  make(map[string]*GlobalShortcut),
		contextShortcuts: make(map[HotkeyContext]map[string]*RegisteredShortcut),
		actions:          make(map[string]HotkeyAction),
		currentMode:      ModeNormal,
		currentContext:   ContextGlobal,
		pendingKeys:      []fyne.KeyName{},
		debugMode:        false,

		vimState: &VimState{
			Mode:        VimNormal,
			PendingKeys: []fyne.KeyName{},
			Marks:       make(map[string]TextPosition),
			JumpList:    []TextPosition{},
		},

		emacsState: &EmacsState{
			PendingKeys:   []fyne.KeyName{},
			KillRing:      []string{},
			KillRingIndex: 0,
		},

		keyLogger: &KeyLogger{
			enabled:    false,
			maxEntries: 1000,
			entries:    []KeyLogEntry{},
		},
	}

	// Инициализируем контекстные карты
	for ctx := ContextGlobal; ctx <= ContextCommandPalette; ctx++ {
		hm.contextShortcuts[ctx] = make(map[string]*RegisteredShortcut)
	}

	// Создаем резолвер конфликтов
	hm.conflictResolver = NewConflictResolver()

	// Определяем режим ввода из конфигурации
	hm.determineInputMode()

	// Регистрируем стандартные действия
	hm.registerStandardActions()

	// Загружаем горячие клавиши из конфигурации
	hm.loadFromConfig()

	// Устанавливаем обработчики событий
	hm.setupEventHandlers()

	return hm
}

// determineInputMode определяет режим ввода из конфигурации
func (hm *HotkeyManager) determineInputMode() {
	if hm.config.KeyBindings.EnableVimBindings {
		hm.currentMode = ModeVim
	} else if hm.config.KeyBindings.EnableEmacsBindings {
		hm.currentMode = ModeEmacs
	} else {
		hm.currentMode = ModeNormal
	}
}

// registerStandardActions регистрирует стандартные действия
func (hm *HotkeyManager) registerStandardActions() {
	// Файловые операции
	hm.actions["new_file"] = hm.actionNewFile
	hm.actions["open_file"] = hm.actionOpenFile
	hm.actions["save_file"] = hm.actionSaveFile
	hm.actions["save_as_file"] = hm.actionSaveAsFile
	hm.actions["close_file"] = hm.actionCloseFile
	hm.actions["close_all"] = hm.actionCloseAll

	// Редактирование
	hm.actions["cut"] = hm.actionCut
	hm.actions["copy"] = hm.actionCopy
	hm.actions["paste"] = hm.actionPaste
	hm.actions["select_all"] = hm.actionSelectAll
	hm.actions["undo"] = hm.actionUndo
	hm.actions["redo"] = hm.actionRedo

	// Поиск и навигация
	hm.actions["find"] = hm.actionFind
	hm.actions["find_next"] = hm.actionFindNext
	hm.actions["find_previous"] = hm.actionFindPrevious
	hm.actions["replace"] = hm.actionReplace
	hm.actions["find_in_files"] = hm.actionFindInFiles
	hm.actions["go_to_line"] = hm.actionGoToLine
	hm.actions["go_to_symbol"] = hm.actionGoToSymbol
	hm.actions["go_to_definition"] = hm.actionGoToDefinition
	hm.actions["file_switcher"] = hm.actionFileSwitcher

	// Интерфейс
	hm.actions["toggle_sidebar"] = hm.actionToggleSidebar
	hm.actions["toggle_minimap"] = hm.actionToggleMinimap
	hm.actions["toggle_terminal"] = hm.actionToggleTerminal
	hm.actions["command_palette"] = hm.actionCommandPalette
	hm.actions["file_explorer"] = hm.actionFileExplorer

	// Форматирование
	hm.actions["format_document"] = hm.actionFormatDocument
	hm.actions["format_selection"] = hm.actionFormatSelection
	hm.actions["comment_line"] = hm.actionCommentLine
	hm.actions["comment_block"] = hm.actionCommentBlock

	// Выделение и курсор
	hm.actions["select_line"] = hm.actionSelectLine
	hm.actions["select_word"] = hm.actionSelectWord
	hm.actions["expand_selection"] = hm.actionExpandSelection
	hm.actions["shrink_selection"] = hm.actionShrinkSelection
	hm.actions["add_cursor_above"] = hm.actionAddCursorAbove
	hm.actions["add_cursor_below"] = hm.actionAddCursorBelow

	// Фолдинг
	hm.actions["fold_block"] = hm.actionFoldBlock
	hm.actions["unfold_block"] = hm.actionUnfoldBlock
	hm.actions["fold_all"] = hm.actionFoldAll
	hm.actions["unfold_all"] = hm.actionUnfoldAll

	// Терминал
	hm.actions["open_terminal"] = hm.actionOpenTerminal
	hm.actions["open_powershell"] = hm.actionOpenPowerShell
	hm.actions["open_cmd"] = hm.actionOpenCMD

	// Сравнение файлов
	hm.actions["compare_files"] = hm.actionCompareFiles

	// Vim специальные действия
	hm.actions["vim_escape"] = hm.actionVimEscape
	hm.actions["vim_insert"] = hm.actionVimInsert
	hm.actions["vim_append"] = hm.actionVimAppend
	hm.actions["vim_delete_line"] = hm.actionVimDeleteLine
	hm.actions["vim_yank_line"] = hm.actionVimYankLine
	hm.actions["vim_paste"] = hm.actionVimPaste

	// Emacs специальные действия
	hm.actions["emacs_kill_line"] = hm.actionEmacsKillLine
	hm.actions["emacs_kill_word"] = hm.actionEmacsKillWord
	hm.actions["emacs_yank"] = hm.actionEmacsYank
	hm.actions["emacs_set_mark"] = hm.actionEmacsSetMark
}

// loadFromConfig загружает горячие клавиши из конфигурации
func (hm *HotkeyManager) loadFromConfig() {
	kb := hm.config.KeyBindings

	// Файловые операции
	hm.registerShortcut("new_file", kb.NewFile, "new_file", ContextGlobal, "File Operations")
	hm.registerShortcut("open_file", kb.OpenFile, "open_file", ContextGlobal, "File Operations")
	hm.registerShortcut("save_file", kb.SaveFile, "save_file", ContextEditor, "File Operations")
	hm.registerShortcut("save_as_file", kb.SaveAsFile, "save_as_file", ContextEditor, "File Operations")
	hm.registerShortcut("close_file", kb.CloseFile, "close_file", ContextEditor, "File Operations")
	hm.registerShortcut("close_all", kb.CloseAll, "close_all", ContextGlobal, "File Operations")

	// Редактирование
	hm.registerShortcut("cut", kb.Cut, "cut", ContextEditor, "Editing")
	hm.registerShortcut("copy", kb.Copy, "copy", ContextEditor, "Editing")
	hm.registerShortcut("paste", kb.Paste, "paste", ContextEditor, "Editing")
	hm.registerShortcut("select_all", kb.SelectAll, "select_all", ContextEditor, "Editing")
	hm.registerShortcut("undo", kb.Undo, "undo", ContextEditor, "Editing")
	hm.registerShortcut("redo", kb.Redo, "redo", ContextEditor, "Editing")

	// Поиск и навигация
	hm.registerShortcut("find", kb.Find, "find", ContextEditor, "Search & Navigation")
	hm.registerShortcut("find_next", kb.FindNext, "find_next", ContextEditor, "Search & Navigation")
	hm.registerShortcut("find_previous", kb.FindPrevious, "find_previous", ContextEditor, "Search & Navigation")
	hm.registerShortcut("replace", kb.Replace, "replace", ContextEditor, "Search & Navigation")
	hm.registerShortcut("find_in_files", kb.FindInFiles, "find_in_files", ContextGlobal, "Search & Navigation")
	hm.registerShortcut("go_to_line", kb.GoToLine, "go_to_line", ContextEditor, "Search & Navigation")
	hm.registerShortcut("go_to_symbol", kb.GoToSymbol, "go_to_symbol", ContextEditor, "Search & Navigation")
	hm.registerShortcut("go_to_definition", kb.GoToDefinition, "go_to_definition", ContextEditor, "Search & Navigation")
	hm.registerShortcut("file_switcher", kb.FileSwitcher, "file_switcher", ContextGlobal, "Search & Navigation")

	// Интерфейс
	hm.registerShortcut("toggle_sidebar", kb.ToggleSidebar, "toggle_sidebar", ContextGlobal, "Interface")
	hm.registerShortcut("toggle_minimap", kb.ToggleMinimap, "toggle_minimap", ContextGlobal, "Interface")
	hm.registerShortcut("toggle_terminal", kb.ToggleTerminal, "toggle_terminal", ContextGlobal, "Interface")
	hm.registerShortcut("command_palette", kb.CommandPalette, "command_palette", ContextGlobal, "Interface")
	hm.registerShortcut("file_explorer", kb.FileExplorer, "file_explorer", ContextGlobal, "Interface")

	// Форматирование
	hm.registerShortcut("format_document", kb.FormatDocument, "format_document", ContextEditor, "Formatting")
	hm.registerShortcut("format_selection", kb.FormatSelection, "format_selection", ContextEditor, "Formatting")
	hm.registerShortcut("comment_line", kb.CommentLine, "comment_line", ContextEditor, "Formatting")
	hm.registerShortcut("comment_block", kb.CommentBlock, "comment_block", ContextEditor, "Formatting")

	// Выделение и курсор
	hm.registerShortcut("select_line", kb.SelectLine, "select_line", ContextEditor, "Selection")
	hm.registerShortcut("select_word", kb.SelectWord, "select_word", ContextEditor, "Selection")
	hm.registerShortcut("expand_selection", kb.ExpandSelection, "expand_selection", ContextEditor, "Selection")
	hm.registerShortcut("shrink_selection", kb.ShrinkSelection, "shrink_selection", ContextEditor, "Selection")
	hm.registerShortcut("add_cursor_above", kb.AddCursorAbove, "add_cursor_above", ContextEditor, "Selection")
	hm.registerShortcut("add_cursor_below", kb.AddCursorBelow, "add_cursor_below", ContextEditor, "Selection")

	// Фолдинг
	hm.registerShortcut("fold_block", kb.FoldBlock, "fold_block", ContextEditor, "Code Folding")
	hm.registerShortcut("unfold_block", kb.UnfoldBlock, "unfold_block", ContextEditor, "Code Folding")
	hm.registerShortcut("fold_all", kb.FoldAll, "fold_all", ContextEditor, "Code Folding")
	hm.registerShortcut("unfold_all", kb.UnfoldAll, "unfold_all", ContextEditor, "Code Folding")

	// Терминал
	hm.registerShortcut("open_terminal", kb.OpenTerminal, "open_terminal", ContextGlobal, "Terminal")
	hm.registerShortcut("open_powershell", kb.OpenPowerShell, "open_powershell", ContextGlobal, "Terminal")
	hm.registerShortcut("open_cmd", kb.OpenCMD, "open_cmd", ContextGlobal, "Terminal")

	// Сравнение файлов
	hm.registerShortcut("compare_files", kb.CompareFiles, "compare_files", ContextGlobal, "Tools")

	// Кастомные привязки
	for action, keyBinding := range kb.CustomBindings {
		if actionFunc, exists := hm.actions[action]; exists {
			hm.registerShortcutWithAction(action, keyBinding, actionFunc, ContextGlobal, "Custom")
		}
	}

	// Загружаем специальные режимы
	if kb.EnableVimBindings {
		hm.loadVimBindings()
	}

	if kb.EnableEmacsBindings {
		hm.loadEmacsBindings()
	}
}

// registerShortcut регистрирует горячую клавишу
func (hm *HotkeyManager) registerShortcut(id, keyBinding, actionName string, context HotkeyContext, category string) {
	if keyBinding == "" {
		return
	}

	action, exists := hm.actions[actionName]
	if !exists {
		fmt.Printf("Warning: Action '%s' not found for shortcut '%s'\n", actionName, id)
		return
	}

	hm.registerShortcutWithAction(id, keyBinding, action, context, category)
}

// registerShortcutWithAction регистрирует горячую клавишу с функцией
func (hm *HotkeyManager) registerShortcutWithAction(id, keyBinding string, action HotkeyAction, context HotkeyContext, category string) {
	shortcut, err := hm.parseKeyBinding(keyBinding)
	if err != nil {
		fmt.Printf("Error parsing key binding '%s': %v\n", keyBinding, err)
		return
	}

	registered := &RegisteredShortcut{
		ID:              id,
		Shortcut:        shortcut,
		Action:          action,
		Context:         context,
		Mode:            hm.currentMode,
		Description:     hm.getActionDescription(id),
		Priority:        hm.getContextPriority(context),
		Enabled:         true,
		Category:        category,
		RequiredContext: context,
		RequiredMode:    ModeNormal, // По умолчанию
	}

	hm.mutex.Lock()
	defer hm.mutex.Unlock()

	// Проверяем конфликты
	if conflicting := hm.findConflictingShortcuts(keyBinding, context); len(conflicting) > 0 {
		hm.resolveConflicts(registered, conflicting)
	}

	// Регистрируем глобально
	hm.shortcuts[id] = registered

	// Регистрируем по контексту
	hm.contextShortcuts[context][keyBinding] = registered

	// Регистрируем в Fyne если это основное окно
	if context == ContextGlobal || context == ContextEditor {
		hm.window.Canvas().AddShortcut(shortcut, hm.createShortcutHandler(registered))
	}
}

// parseKeyBinding парсит строку горячей клавиши
func (hm *HotkeyManager) parseKeyBinding(keyBinding string) (*desktop.CustomShortcut, error) {
	if keyBinding == "" {
		return nil, fmt.Errorf("empty key binding")
	}

	// Разбираем комбинацию клавиш
	parts := strings.Split(keyBinding, "+")
	if len(parts) == 0 {
		return nil, fmt.Errorf("invalid key binding format")
	}

	var modifiers fyne.KeyModifier
	var key fyne.KeyName

	// Обрабатываем модификаторы
	for i, part := range parts {
		part = strings.TrimSpace(part)

		if i == len(parts)-1 {
			// Последняя часть - основная клавиша
			key = hm.parseKeyName(part)
			if key == "" {
				return nil, fmt.Errorf("invalid key name: %s", part)
			}
		} else {
			// Модификаторы
			switch strings.ToLower(part) {
			case "ctrl", "control":
				modifiers |= fyne.KeyModifierControl
			case "alt":
				modifiers |= fyne.KeyModifierAlt
			case "shift":
				modifiers |= fyne.KeyModifierShift
			case "super", "cmd", "win":
				modifiers |= fyne.KeyModifierSuper
			default:
				return nil, fmt.Errorf("unknown modifier: %s", part)
			}
		}
	}

	if key == "" {
		return nil, fmt.Errorf("no key specified")
	}

	return &desktop.CustomShortcut{
		KeyName:  key,
		Modifier: modifiers,
	}, nil
}

// parseKeyName преобразует строку в fyne.KeyName
func (hm *HotkeyManager) parseKeyName(keyStr string) fyne.KeyName {
	keyStr = strings.ToLower(strings.TrimSpace(keyStr))

	// Специальные клавиши
	specialKeys := map[string]fyne.KeyName{
		"space":     fyne.KeySpace,
		"enter":     fyne.KeyReturn,
		"return":    fyne.KeyReturn,
		"tab":       fyne.KeyTab,
		"escape":    fyne.KeyEscape,
		"esc":       fyne.KeyEscape,
		"backspace": fyne.KeyBackspace,
		"delete":    fyne.KeyDelete,
		"insert":    fyne.KeyInsert,
		"home":      fyne.KeyHome,
		"end":       fyne.KeyEnd,
		"pageup":    fyne.KeyPageUp,
		"pagedown":  fyne.KeyPageDown,
		"up":        fyne.KeyUp,
		"down":      fyne.KeyDown,
		"left":      fyne.KeyLeft,
		"right":     fyne.KeyRight,
		"`":         fyne.KeyBackTick,
		"-":         fyne.KeyMinus,
		"=":         fyne.KeyEqual,
		"[":         fyne.KeyLeftBracket,
		"]":         fyne.KeyRightBracket,
		"\\":        fyne.KeyBackslash,
		";":         fyne.KeySemicolon,
		"'":         fyne.KeyApostrophe,
		",":         fyne.KeyComma,
		".":         fyne.KeyPeriod,
		"/":         fyne.KeySlash,
	}

	if specialKey, exists := specialKeys[keyStr]; exists {
		return specialKey
	}

	// Функциональные клавиши
	if strings.HasPrefix(keyStr, "f") && len(keyStr) >= 2 {
		switch keyStr {
		case "f1":
			return fyne.KeyF1
		case "f2":
			return fyne.KeyF2
		case "f3":
			return fyne.KeyF3
		case "f4":
			return fyne.KeyF4
		case "f5":
			return fyne.KeyF5
		case "f6":
			return fyne.KeyF6
		case "f7":
			return fyne.KeyF7
		case "f8":
			return fyne.KeyF8
		case "f9":
			return fyne.KeyF9
		case "f10":
			return fyne.KeyF10
		case "f11":
			return fyne.KeyF11
		case "f12":
			return fyne.KeyF12
		}
	}

	// Обычные символы
	if len(keyStr) == 1 {
		char := keyStr[0]
		if char >= 'a' && char <= 'z' {
			return fyne.KeyName(char - 'a' + 'A') // Преобразуем в верхний регистр
		}
		if char >= '0' && char <= '9' {
			return fyne.KeyName(char)
		}
	}

	return ""
}

// setupEventHandlers устанавливает обработчики событий
func (hm *HotkeyManager) setupEventHandlers() {
	// Для десктопных приложений используем низкоуровневые события,
	// чтобы отслеживать нажатия и отпускания модификаторов
	if deskCanvas, ok := hm.window.Canvas().(desktop.Canvas); ok {
		deskCanvas.SetOnKeyDown(func(ev *fyne.KeyEvent) {
			hm.updateModifierState(ev, true)
			hm.handleKeyEvent(ev)
		})
		deskCanvas.SetOnKeyUp(func(ev *fyne.KeyEvent) {
			hm.updateModifierState(ev, false)
		})
		hm.window.Canvas().SetOnTypedRune(func(r rune) {
			hm.handleRuneEvent(r)
		})
	} else {
		// Для других платформ используем стандартные обработчики
		hm.window.Canvas().SetOnTypedKey(func(key *fyne.KeyEvent) {
			hm.updateModifierState(key, true)
			hm.handleKeyEvent(key)
			hm.updateModifierState(key, false)
		})
		hm.window.Canvas().SetOnTypedRune(func(r rune) {
			hm.handleRuneEvent(r)
		})
	}
}

// handleKeyEvent обрабатывает нажатие клавиши
func (hm *HotkeyManager) handleKeyEvent(event *fyne.KeyEvent) {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()

	hm.handleFocusChanged(hm.window.Canvas().Focused())

	// Логируем нажатие
	if hm.keyLogger.enabled {
		hm.keyLogger.logKey(event.Name, hm.getModifierState(), hm.currentContext, hm.currentMode)
	}

	// Обрабатываем в зависимости от режима
	switch hm.currentMode {
	case ModeVim:
		if hm.handleVimKeyEvent(event) {
			return
		}
	case ModeEmacs:
		if hm.handleEmacsKeyEvent(event) {
			return
		}
	}

	// Обрабатываем мультиклавишные комбинации
	if hm.handleMultiKeySequence(event) {
		return
	}

	// Обычная обработка горячих клавиш
	hm.handleNormalKeyEvent(event)
}

// handleVimKeyEvent обрабатывает клавиши в Vim режиме
func (hm *HotkeyManager) handleVimKeyEvent(event *fyne.KeyEvent) bool {
	state := hm.vimState

	// ESC всегда возвращает в Normal режим
	if event.Name == fyne.KeyEscape {
		state.Mode = VimNormal
		state.PendingKeys = []fyne.KeyName{}
		state.PendingOperator = ""
		state.PendingMotion = ""
		state.Count = 0
		return true
	}

	switch state.Mode {
	case VimNormal:
		return hm.handleVimNormalMode(event)
	case VimInsert:
		return hm.handleVimInsertMode(event)
	case VimVisual:
		return hm.handleVimVisualMode(event)
	case VimCommand:
		return hm.handleVimCommandMode(event)
	}

	return false
}

// handleVimNormalMode обрабатывает Normal режим Vim
func (hm *HotkeyManager) handleVimNormalMode(event *fyne.KeyEvent) bool {
	state := hm.vimState

	// Добавляем клавишу к ожидающим
	state.PendingKeys = append(state.PendingKeys, event.Name)

	// Проверяем команды
	keySequence := hm.keySequenceToString(state.PendingKeys)

	// Движения курсора
	switch keySequence {
	case "h":
		hm.executeAction("vim_move_left")
		state.PendingKeys = []fyne.KeyName{}
		return true
	case "j":
		hm.executeAction("vim_move_down")
		state.PendingKeys = []fyne.KeyName{}
		return true
	case "k":
		hm.executeAction("vim_move_up")
		state.PendingKeys = []fyne.KeyName{}
		return true
	case "l":
		hm.executeAction("vim_move_right")
		state.PendingKeys = []fyne.KeyName{}
		return true
	case "w":
		hm.executeAction("vim_word_forward")
		state.PendingKeys = []fyne.KeyName{}
		return true
	case "b":
		hm.executeAction("vim_word_backward")
		state.PendingKeys = []fyne.KeyName{}
		return true
	case "0":
		hm.executeAction("vim_line_start")
		state.PendingKeys = []fyne.KeyName{}
		return true
	case "$":
		hm.executeAction("vim_line_end")
		state.PendingKeys = []fyne.KeyName{}
		return true

	// Режимы
	case "i":
		state.Mode = VimInsert
		hm.executeAction("vim_insert")
		state.PendingKeys = []fyne.KeyName{}
		return true
	case "a":
		state.Mode = VimInsert
		hm.executeAction("vim_append")
		state.PendingKeys = []fyne.KeyName{}
		return true
	case "o":
		state.Mode = VimInsert
		hm.executeAction("vim_new_line")
		state.PendingKeys = []fyne.KeyName{}
		return true
	case "v":
		state.Mode = VimVisual
		hm.executeAction("vim_visual")
		state.PendingKeys = []fyne.KeyName{}
		return true

	// Операции
	case "dd":
		hm.executeAction("vim_delete_line")
		state.PendingKeys = []fyne.KeyName{}
		return true
	case "yy":
		hm.executeAction("vim_yank_line")
		state.PendingKeys = []fyne.KeyName{}
		return true
	case "p":
		hm.executeAction("vim_paste")
		state.PendingKeys = []fyne.KeyName{}
		return true
	case "u":
		hm.executeAction("undo")
		state.PendingKeys = []fyne.KeyName{}
		return true
	case ":":
		state.Mode = VimCommand
		state.ExCommandMode = true
		state.ExCommandBuffer = ""
		state.PendingKeys = []fyne.KeyName{}
		return true
	}

	// Если последовательность стала слишком длинной, сбрасываем
	if len(state.PendingKeys) > 3 {
		state.PendingKeys = []fyne.KeyName{}
	}

	return false
}

// handleVimInsertMode обрабатывает Insert режим Vim
func (hm *HotkeyManager) handleVimInsertMode(event *fyne.KeyEvent) bool {
	// В Insert режиме обычно пропускаем обработку, кроме ESC
	if event.Name == fyne.KeyEscape {
		hm.vimState.Mode = VimNormal
		return true
	}

	return false // Пропускаем в обычную обработку
}

// handleVimVisualMode обрабатывает Visual режим Vim
func (hm *HotkeyManager) handleVimVisualMode(event *fyne.KeyEvent) bool {
	state := hm.vimState

	// Выход из Visual режима
	if event.Name == fyne.KeyEscape {
		state.Mode = VimNormal
		hm.executeAction("vim_clear_selection")
		return true
	}

	// Операции с выделением
	switch event.Name {
	case fyne.KeyName('d'), fyne.KeyName('D'):
		hm.executeAction("vim_delete_selection")
		state.Mode = VimNormal
		return true
	case fyne.KeyName('y'), fyne.KeyName('Y'):
		hm.executeAction("vim_yank_selection")
		state.Mode = VimNormal
		return true
	case fyne.KeyName('c'), fyne.KeyName('C'):
		hm.executeAction("vim_change_selection")
		state.Mode = VimInsert
		return true
	}

	return false
}

// handleVimCommandMode обрабатывает Command режим Vim
func (hm *HotkeyManager) handleVimCommandMode(event *fyne.KeyEvent) bool {
	state := hm.vimState

	if event.Name == fyne.KeyEscape {
		state.Mode = VimNormal
		state.ExCommandMode = false
		state.ExCommandBuffer = ""
		return true
	}

	if event.Name == fyne.KeyReturn {
		// Выполняем команду
		hm.executeVimCommand(state.ExCommandBuffer)
		state.Mode = VimNormal
		state.ExCommandMode = false
		state.ExCommandBuffer = ""
		return true
	}

	if event.Name == fyne.KeyBackspace {
		if len(state.ExCommandBuffer) > 0 {
			state.ExCommandBuffer = state.ExCommandBuffer[:len(state.ExCommandBuffer)-1]
		}
		return true
	}

	// Добавляем символ к команде
	if len(string(event.Name)) == 1 {
		state.ExCommandBuffer += string(event.Name)
		return true
	}

	return false
}

// handleEmacsKeyEvent обрабатывает клавиши в Emacs режиме
func (hm *HotkeyManager) handleEmacsKeyEvent(event *fyne.KeyEvent) bool {
	state := hm.emacsState

	// Обрабатываем последовательности клавиш
	state.PendingKeys = append(state.PendingKeys, event.Name)
	keySequence := hm.keySequenceToString(state.PendingKeys)

	// Основные команды Emacs
	switch keySequence {
	case "C-f":
		hm.executeAction("emacs_forward_char")
		state.PendingKeys = []fyne.KeyName{}
		return true
	case "C-b":
		hm.executeAction("emacs_backward_char")
		state.PendingKeys = []fyne.KeyName{}
		return true
	case "C-n":
		hm.executeAction("emacs_next_line")
		state.PendingKeys = []fyne.KeyName{}
		return true
	case "C-p":
		hm.executeAction("emacs_previous_line")
		state.PendingKeys = []fyne.KeyName{}
		return true
	case "C-a":
		hm.executeAction("emacs_beginning_of_line")
		state.PendingKeys = []fyne.KeyName{}
		return true
	case "C-e":
		hm.executeAction("emacs_end_of_line")
		state.PendingKeys = []fyne.KeyName{}
		return true
	case "C-k":
		hm.executeAction("emacs_kill_line")
		state.PendingKeys = []fyne.KeyName{}
		return true
	case "C-y":
		hm.executeAction("emacs_yank")
		state.PendingKeys = []fyne.KeyName{}
		return true
	case "C-w":
		hm.executeAction("emacs_kill_region")
		state.PendingKeys = []fyne.KeyName{}
		return true
	case "C-SPC":
		hm.executeAction("emacs_set_mark")
		state.PendingKeys = []fyne.KeyName{}
		return true
	case "C-s":
		hm.executeAction("emacs_search_forward")
		state.PendingKeys = []fyne.KeyName{}
		return true
	case "C-r":
		hm.executeAction("emacs_search_backward")
		state.PendingKeys = []fyne.KeyName{}
		return true
	case "C-x C-f":
		hm.executeAction("open_file")
		state.PendingKeys = []fyne.KeyName{}
		return true
	case "C-x C-s":
		hm.executeAction("save_file")
		state.PendingKeys = []fyne.KeyName{}
		return true
	}

	// Сбрасываем если последовательность стала слишком длинной
	if len(state.PendingKeys) > 3 {
		state.PendingKeys = []fyne.KeyName{}
	}

	return false
}

// handleMultiKeySequence обрабатывает мультиклавишные комбинации
func (hm *HotkeyManager) handleMultiKeySequence(event *fyne.KeyEvent) bool {
	// Добавляем клавишу к ожидающим
	hm.pendingKeys = append(hm.pendingKeys, event.Name)

	keySequence := hm.keySequenceToString(hm.pendingKeys)

	// Проверяем зарегистрированные последовательности
	for _, shortcut := range hm.shortcuts {
		if shortcut.Enabled && hm.isContextActive(shortcut.Context) {
			shortcutSequence := hm.shortcutToString(shortcut.Shortcut)

			if keySequence == shortcutSequence {
				// Найдена полная последовательность
				hm.executeShortcut(shortcut)
				hm.clearPendingKeys()
				return true
			}

			if strings.HasPrefix(shortcutSequence, keySequence) {
				// Частичное совпадение - ждем еще клавиш
				hm.resetPendingTimeout()
				return true
			}
		}
	}

	// Сбрасываем если нет совпадений
	hm.clearPendingKeys()
	return false
}

// handleNormalKeyEvent обрабатывает обычные горячие клавиши
func (hm *HotkeyManager) handleNormalKeyEvent(event *fyne.KeyEvent) {
	// Ищем прямые совпадения
	for _, shortcut := range hm.contextShortcuts[hm.currentContext] {
		if shortcut.Enabled && hm.matchesKeyEvent(shortcut.Shortcut, event) {
			if hm.executeShortcut(shortcut) {
				return
			}
		}
	}

	// Проверяем глобальные горячие клавиши
	for _, shortcut := range hm.contextShortcuts[ContextGlobal] {
		if shortcut.Enabled && hm.matchesKeyEvent(shortcut.Shortcut, event) {
			hm.executeShortcut(shortcut)
			return
		}
	}
}

// handleRuneEvent обрабатывает ввод символа
func (hm *HotkeyManager) handleRuneEvent(r rune) {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()

	hm.handleFocusChanged(hm.window.Canvas().Focused())

	// В Vim режиме обрабатываем символы для команд
	if hm.currentMode == ModeVim && hm.vimState.Mode == VimNormal {
		// Числовые префиксы
		if r >= '0' && r <= '9' {
			if hm.vimState.Count == 0 && r == '0' {
				// 0 как команда перехода к началу строки
				hm.executeAction("vim_line_start")
			} else {
				// Числовой префикс
				hm.vimState.Count = hm.vimState.Count*10 + int(r-'0')
			}
			return
		}
	}

	// В Command режиме Vim добавляем символы к команде
	if hm.currentMode == ModeVim && hm.vimState.ExCommandMode {
		hm.vimState.ExCommandBuffer += string(r)
		return
	}
}

// Методы выполнения действий

// executeAction выполняет действие по имени
func (hm *HotkeyManager) executeAction(actionName string) bool {
	if action, exists := hm.actions[actionName]; exists {
		return action(hm.currentContext)
	}
	return false
}

// executeActionWithParam выполняет действие с параметром
func (hm *HotkeyManager) executeActionWithParam(actionName string, param interface{}) bool {
	switch actionName {
	case "open_file":
		if path, ok := param.(string); ok && hm.app != nil {
			hm.app.loadFile(path)
			hm.app.currentFile = path
			hm.app.updateTitle()
			hm.app.addToRecentFiles(path)
			return true
		}
	}

	if action, exists := hm.actions[actionName]; exists {
		return action(hm.currentContext)
	}
	return false
}

// executeShortcut выполняет зарегистрированную горячую клавишу
func (hm *HotkeyManager) executeShortcut(shortcut *RegisteredShortcut) bool {
	// Проверяем условия активации
	if !hm.canExecuteShortcut(shortcut) {
		return false
	}

	// Обновляем статистику
	shortcut.LastUsed = time.Now()
	shortcut.UseCount++

	// Выполняем действие
	result := shortcut.Action(hm.currentContext)

	// Логируем выполнение
	if hm.keyLogger.enabled {
		hm.keyLogger.logAction(shortcut.ID, result)
	}

	return result
}

// executeVimCommand выполняет Ex команду Vim
func (hm *HotkeyManager) executeVimCommand(command string) {
	command = strings.TrimSpace(command)

	switch {
	case command == "q" || command == "quit":
		hm.executeAction("close_file")
	case command == "w" || command == "write":
		hm.executeAction("save_file")
	case command == "wq" || command == "x":
		hm.executeAction("save_file")
		hm.executeAction("close_file")
	case command == "q!" || command == "quit!":
		hm.executeAction("close_file") // Без сохранения
	case strings.HasPrefix(command, "e ") || strings.HasPrefix(command, "edit "):
		// Открыть файл
		filename := strings.TrimSpace(command[1:])
		if filename != "" {
			hm.executeActionWithParam("open_file", filename)
		}
	case command == "split":
		hm.executeAction("split_horizontal")
	case command == "vsplit":
		hm.executeAction("split_vertical")
	default:
		fmt.Printf("Unknown vim command: %s\n", command)
	}
}

// Вспомогательные методы

// keySequenceToString преобразует последовательность клавиш в строку
func (hm *HotkeyManager) keySequenceToString(keys []fyne.KeyName) string {
	var parts []string
	for _, key := range keys {
		parts = append(parts, string(key))
	}
	return strings.Join(parts, " ")
}

// shortcutToString преобразует shortcut в строку
func (hm *HotkeyManager) shortcutToString(shortcut *desktop.CustomShortcut) string {
	var parts []string

	if shortcut.Modifier&fyne.KeyModifierControl != 0 {
		parts = append(parts, "Ctrl")
	}
	if shortcut.Modifier&fyne.KeyModifierAlt != 0 {
		parts = append(parts, "Alt")
	}
	if shortcut.Modifier&fyne.KeyModifierShift != 0 {
		parts = append(parts, "Shift")
	}
	if shortcut.Modifier&fyne.KeyModifierSuper != 0 {
		parts = append(parts, "Super")
	}

	parts = append(parts, string(shortcut.KeyName))

	return strings.Join(parts, "+")
}

// matchesKeyEvent проверяет соответствие shortcut событию клавиши
func (hm *HotkeyManager) matchesKeyEvent(shortcut *desktop.CustomShortcut, event *fyne.KeyEvent) bool {
	return shortcut.KeyName == event.Name // Fyne автоматически проверяет модификаторы
}

// isContextActive проверяет активность контекста
func (hm *HotkeyManager) isContextActive(context HotkeyContext) bool {
	return context == ContextGlobal || context == hm.currentContext
}

// canExecuteShortcut проверяет возможность выполнения горячей клавиши
func (hm *HotkeyManager) canExecuteShortcut(shortcut *RegisteredShortcut) bool {
	if !shortcut.Enabled {
		return false
	}

	if shortcut.RequiredMode != ModeNormal && shortcut.RequiredMode != hm.currentMode {
		return false
	}

	if !hm.isContextActive(shortcut.RequiredContext) {
		return false
	}

	return true
}

// createShortcutHandler создает обработчик для Fyne shortcut
func (hm *HotkeyManager) createShortcutHandler(shortcut *RegisteredShortcut) func(fyne.Shortcut) {
	return func(fyne.Shortcut) {
		hm.executeShortcut(shortcut)
	}
}

// updateModifierState обновляет состояние модификаторов
func (hm *HotkeyManager) updateModifierState(event *fyne.KeyEvent, pressed bool) {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()

	switch event.Name {
	case desktop.KeyControlLeft, desktop.KeyControlRight:
		hm.modifierState.Ctrl = pressed
	case desktop.KeyShiftLeft, desktop.KeyShiftRight:
		hm.modifierState.Shift = pressed
	case desktop.KeyAltLeft, desktop.KeyAltRight:
		hm.modifierState.Alt = pressed
	case desktop.KeySuperLeft, desktop.KeySuperRight:
		hm.modifierState.Super = pressed
	}
}

// getModifierState возвращает текущее состояние модификаторов
func (hm *HotkeyManager) getModifierState() ModifierState {
	return hm.modifierState
}

// handleFocusChanged обрабатывает изменение фокуса
func (hm *HotkeyManager) handleFocusChanged(obj fyne.Focusable) {
	hm.focusedWidget = obj

	// Определяем контекст по типу виджета
	oldContext := hm.currentContext

	switch obj.(type) {
	case *EditorWidget:
		hm.currentContext = ContextEditor
	case *SidebarWidget:
		hm.currentContext = ContextSidebar
	case *MinimapWidget:
		hm.currentContext = ContextMinimap
	default:
		hm.currentContext = ContextGlobal
	}

	// Уведомляем о смене контекста
	if oldContext != hm.currentContext {
		hm.onContextChanged(oldContext, hm.currentContext)
	}
}

// onContextChanged вызывается при смене контекста
func (hm *HotkeyManager) onContextChanged(oldContext, newContext HotkeyContext) {
	if hm.debugMode {
		fmt.Printf("Context changed: %s -> %s\n",
			hm.contextToString(oldContext),
			hm.contextToString(newContext))
	}
}

// Методы управления pending keys

// clearPendingKeys очищает ожидающие клавиши
func (hm *HotkeyManager) clearPendingKeys() {
	hm.pendingKeys = []fyne.KeyName{}
	if hm.pendingTimeout != nil {
		hm.pendingTimeout.Stop()
		hm.pendingTimeout = nil
	}
}

// resetPendingTimeout сбрасывает таймер ожидания
func (hm *HotkeyManager) resetPendingTimeout() {
	if hm.pendingTimeout != nil {
		hm.pendingTimeout.Stop()
	}

	hm.pendingTimeout = time.AfterFunc(1*time.Second, func() {
		hm.clearPendingKeys()
	})
}

// Методы разрешения конфликтов

// findConflictingShortcuts находит конфликтующие горячие клавиши
func (hm *HotkeyManager) findConflictingShortcuts(keyBinding string, context HotkeyContext) []*RegisteredShortcut {
	var conflicts []*RegisteredShortcut

	for _, shortcut := range hm.shortcuts {
		if shortcut.Context == context && hm.shortcutToString(shortcut.Shortcut) == keyBinding {
			conflicts = append(conflicts, shortcut)
		}
	}

	return conflicts
}

// resolveConflicts разрешает конфликты горячих клавиш
func (hm *HotkeyManager) resolveConflicts(newShortcut *RegisteredShortcut, conflicts []*RegisteredShortcut) {
	resolution := hm.conflictResolver.resolve(newShortcut, conflicts)

	switch resolution {
	case ResolutionHighestPriority:
		// Оставляем с наивысшим приоритетом
		for _, conflict := range conflicts {
			if conflict.Priority < newShortcut.Priority {
				conflict.Enabled = false
			}
		}
	case ResolutionDisable:
		// Отключаем все конфликтующие
		for _, conflict := range conflicts {
			conflict.Enabled = false
		}
	}
}

// Публичные методы API

// SetContext устанавливает текущий контекст
func (hm *HotkeyManager) SetContext(context HotkeyContext) {
	oldContext := hm.currentContext
	hm.currentContext = context
	hm.onContextChanged(oldContext, context)
}

// GetContext возвращает текущий контекст
func (hm *HotkeyManager) GetContext() HotkeyContext {
	return hm.currentContext
}

// SetMode устанавливает режим ввода
func (hm *HotkeyManager) SetMode(mode InputMode) {
	hm.currentMode = mode

	// Сбрасываем состояния при смене режима
	if mode != ModeVim {
		hm.vimState = &VimState{
			Mode:        VimNormal,
			PendingKeys: []fyne.KeyName{},
			Marks:       make(map[string]TextPosition),
			JumpList:    []TextPosition{},
		}
	}

	if mode != ModeEmacs {
		hm.emacsState = &EmacsState{
			PendingKeys:   []fyne.KeyName{},
			KillRing:      []string{},
			KillRingIndex: 0,
		}
	}
}

// GetMode возвращает текущий режим ввода
func (hm *HotkeyManager) GetMode() InputMode {
	return hm.currentMode
}

// EnableDebugMode включает режим отладки
func (hm *HotkeyManager) EnableDebugMode(enabled bool) {
	hm.debugMode = enabled
	hm.keyLogger.enabled = enabled
}

// GetShortcuts возвращает все зарегистрированные горячие клавиши
func (hm *HotkeyManager) GetShortcuts() map[string]*RegisteredShortcut {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()

	// Возвращаем копию для безопасности
	result := make(map[string]*RegisteredShortcut)
	for k, v := range hm.shortcuts {
		result[k] = v
	}

	return result
}

// UpdateKeyBinding обновляет привязку клавиши
func (hm *HotkeyManager) UpdateKeyBinding(id, newKeyBinding string) error {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()

	shortcut, exists := hm.shortcuts[id]
	if !exists {
		return fmt.Errorf("shortcut with id '%s' not found", id)
	}

	newShortcut, err := hm.parseKeyBinding(newKeyBinding)
	if err != nil {
		return fmt.Errorf("invalid key binding: %v", err)
	}

	// Удаляем старую привязку
	oldKeyBinding := hm.shortcutToString(shortcut.Shortcut)
	delete(hm.contextShortcuts[shortcut.Context], oldKeyBinding)

	// Добавляем новую
	shortcut.Shortcut = newShortcut
	hm.contextShortcuts[shortcut.Context][newKeyBinding] = shortcut

	// Обновляем в Fyne
	if shortcut.Context == ContextGlobal || shortcut.Context == ContextEditor {
		hm.window.Canvas().RemoveShortcut(shortcut.Shortcut)
		hm.window.Canvas().AddShortcut(newShortcut, hm.createShortcutHandler(shortcut))
	}

	return nil
}

// RegisterCustomAction регистрирует пользовательское действие
func (hm *HotkeyManager) RegisterCustomAction(name string, action HotkeyAction) {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()

	hm.actions[name] = action
}

// Реализация стандартных действий

// Файловые операции
func (hm *HotkeyManager) actionNewFile(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Проверяем несохраненные изменения
	if hm.app.editor.IsDirty() {
		dialog.ShowConfirm("Unsaved Changes",
			"Do you want to save the current file?",
			func(save bool) {
				if save {
					hm.app.saveFile()
				}
				// Создаем новый файл
				hm.app.editor.Clear()
				hm.app.currentFile = ""
				hm.app.updateTitle()
			}, hm.window)
	} else {
		// Создаем новый файл
		hm.app.editor.Clear()
		hm.app.currentFile = ""
		hm.app.updateTitle()
	}

	return true
}

func (hm *HotkeyManager) actionOpenFile(context HotkeyContext) bool {
	if hm.app == nil {
		return false
	}

	// Открываем диалог выбора файла
	dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, hm.window)
			return
		}
		if reader == nil {
			return
		}
		defer reader.Close()

		// Читаем содержимое файла
		data, err := ioutil.ReadAll(reader)
		if err != nil {
			dialog.ShowError(err, hm.window)
			return
		}

		// Загружаем в редактор
		hm.app.editor.SetContent(string(data))
		hm.app.editor.SetFilePath(reader.URI().Path())
		hm.app.currentFile = reader.URI().Path()
		hm.app.updateTitle()
		hm.app.addToRecentFiles(reader.URI().Path())
	}, hm.window)

	return true
}

func (hm *HotkeyManager) actionSaveFile(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	if hm.app.currentFile == "" {
		return hm.actionSaveAsFile(context)
	}

	// Сохраняем файл
	err := hm.app.editor.SaveFile()
	if err != nil {
		dialog.ShowError(err, hm.window)
		return false
	}

	hm.app.updateTitle()
	return true
}

func (hm *HotkeyManager) actionSaveAsFile(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Показываем диалог сохранения
	dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, hm.window)
			return
		}
		if writer == nil {
			return
		}
		defer writer.Close()

		// Записываем содержимое
		content := hm.app.editor.GetContent()
		_, err = writer.Write([]byte(content))
		if err != nil {
			dialog.ShowError(err, hm.window)
			return
		}

		// Обновляем путь к файлу
		hm.app.currentFile = writer.URI().Path()
		hm.app.editor.SetFilePath(writer.URI().Path())
		hm.app.updateTitle()
		hm.app.addToRecentFiles(writer.URI().Path())
	}, hm.window)

	return true
}

func (hm *HotkeyManager) actionCloseFile(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Проверяем несохраненные изменения
	if hm.app.editor.IsDirty() {
		dialog.ShowConfirm("Unsaved Changes",
			"Do you want to save before closing?",
			func(save bool) {
				if save {
					hm.actionSaveFile(context)
				}
				// Закрываем файл
				hm.app.editor.Clear()
				hm.app.currentFile = ""
				hm.app.updateTitle()
			}, hm.window)
	} else {
		// Закрываем файл
		hm.app.editor.Clear()
		hm.app.currentFile = ""
		hm.app.updateTitle()
	}

	return true
}

func (hm *HotkeyManager) actionCloseAll(context HotkeyContext) bool {
	// В текущей реализации работаем с одним файлом
	return hm.actionCloseFile(context)
}

// Редактирование
func (hm *HotkeyManager) actionCut(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Получаем выделенный текст
	selectedText := hm.app.editor.GetSelectedText()
	if selectedText == "" {
		return false
	}

	// Копируем в буфер обмена
	hm.window.Clipboard().SetContent(selectedText)

	// Удаляем выделенный текст
	hm.app.editor.ReplaceSelection("")

	return true
}

func (hm *HotkeyManager) actionCopy(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Получаем выделенный текст
	selectedText := hm.app.editor.GetSelectedText()
	if selectedText == "" {
		// Если ничего не выделено, копируем текущую строку
		selectedText = hm.app.editor.GetCurrentLine()
	}

	// Копируем в буфер обмена
	hm.window.Clipboard().SetContent(selectedText)

	return true
}

func (hm *HotkeyManager) actionPaste(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Получаем содержимое буфера обмена
	content := hm.window.Clipboard().Content()
	if content == "" {
		return false
	}

	// Вставляем текст
	cmd := &InsertTextCommand{
		text:     content,
		position: hm.app.editor.GetCursorPosition(),
	}
	hm.app.editor.ExecuteCommand(cmd)

	return true
}

func (hm *HotkeyManager) actionSelectAll(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Выделяем весь текст
	hm.app.editor.SelectAll()
	return true
}

func (hm *HotkeyManager) actionUndo(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Отменяем последнее действие
	hm.app.editor.Undo()
	return true
}

func (hm *HotkeyManager) actionRedo(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Повторяем отмененное действие
	hm.app.editor.Redo()
	return true
}

// Поиск и навигация
func (hm *HotkeyManager) actionFind(context HotkeyContext) bool {
	if hm.app == nil {
		return false
	}

	// Показываем диалог поиска
	hm.app.showFind()
	return true
}

func (hm *HotkeyManager) actionFindNext(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Ищем следующее вхождение
	if hm.app.lastSearchText != "" {
		found := hm.app.editor.FindNext(hm.app.lastSearchText)
		if !found {
			dialog.ShowInformation("Find", "No more occurrences found", hm.window)
		}
	}

	return true
}

func (hm *HotkeyManager) actionFindPrevious(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Ищем предыдущее вхождение
	if hm.app.lastSearchText != "" {
		found := hm.app.editor.FindPrevious(hm.app.lastSearchText)
		if !found {
			dialog.ShowInformation("Find", "No more occurrences found", hm.window)
		}
	}

	return true
}

func (hm *HotkeyManager) actionReplace(context HotkeyContext) bool {
	if hm.app == nil {
		return false
	}

	// Показываем диалог замены
	hm.app.showReplace()
	return true
}

func (hm *HotkeyManager) actionFindInFiles(context HotkeyContext) bool {
	if hm.app == nil {
		return false
	}

	// Показываем диалог поиска в файлах
	hm.app.showFindInFiles()
	return true
}

func (hm *HotkeyManager) actionGoToLine(context HotkeyContext) bool {
	if hm.app == nil {
		return false
	}

	// Показываем диалог перехода к строке
	hm.app.showGoToLine()
	return true
}

func (hm *HotkeyManager) actionGoToSymbol(context HotkeyContext) bool {
	if hm.app == nil {
		return false
	}

	// Показываем диалог перехода к символу
	hm.app.showGoToSymbol()
	return true
}

func (hm *HotkeyManager) actionGoToDefinition(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Получаем слово под курсором
	word := hm.app.editor.GetWordAtCursor()
	if word == "" {
		return false
	}

	// Ищем определение
	definition := hm.app.editor.FindDefinition(word)
	if definition != nil {
		// Переходим к определению
		hm.app.editor.GoToPosition(definition.Line, definition.Column)
		return true
	}

	dialog.ShowInformation("Go to Definition",
		fmt.Sprintf("Definition for '%s' not found", word), hm.window)
	return false
}

// Интерфейс
func (hm *HotkeyManager) actionToggleSidebar(context HotkeyContext) bool {
	if hm.app == nil {
		return false
	}

	hm.app.toggleSidebar()
	return true
}

func (hm *HotkeyManager) actionToggleMinimap(context HotkeyContext) bool {
	if hm.app == nil {
		return false
	}

	hm.app.toggleMinimap()
	return true
}

func (hm *HotkeyManager) actionToggleTerminal(context HotkeyContext) bool {
	if hm.app == nil || hm.app.terminalMgr == nil {
		return false
	}

	// Переключаем видимость терминала
	if hm.app.terminalMgr.IsVisible() {
		hm.app.terminalMgr.Hide()
	} else {
		hm.app.terminalMgr.Show()
	}

	return true
}

func (hm *HotkeyManager) actionCommandPalette(context HotkeyContext) bool {
	if hm.app == nil {
		return false
	}

	hm.app.showCommandPalette()
	return true
}

func (hm *HotkeyManager) actionFileSwitcher(context HotkeyContext) bool {
	if hm.app == nil {
		return false
	}

	hm.app.showFileSwitcher()
	return true
}

func (hm *HotkeyManager) actionFileExplorer(context HotkeyContext) bool {
	if hm.app == nil {
		return false
	}

	// Фокусируемся на файловом проводнике
	hm.app.focusFileExplorer()
	return true
}

// Форматирование
func (hm *HotkeyManager) actionFormatDocument(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Форматируем весь документ
	language := hm.app.editor.GetLanguage()
	cmd := &FormatCodeCommand{language: language}
	err := cmd.Execute(hm.app.editor)
	if err != nil {
		dialog.ShowError(err, hm.window)
		return false
	}

	return true
}

func (hm *HotkeyManager) actionFormatSelection(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Получаем выделенный текст
	selectedText := hm.app.editor.GetSelectedText()
	if selectedText == "" {
		return false
	}

	// Форматируем выделение
	language := hm.app.editor.GetLanguage()
	formatted, err := formatCode(selectedText, language)
	if err != nil {
		dialog.ShowError(err, hm.window)
		return false
	}

	// Заменяем выделенный текст отформатированным
	hm.app.editor.ReplaceSelection(formatted)
	return true
}

func (hm *HotkeyManager) actionCommentLine(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Комментируем/раскомментируем текущую строку
	language := hm.app.editor.GetLanguage()
	lineComment := getLineCommentSymbol(language)
	if lineComment == "" {
		return false
	}

	currentLine := hm.app.editor.GetCurrentLine()
	trimmed := strings.TrimSpace(currentLine)

	var newLine string
	if strings.HasPrefix(trimmed, lineComment) {
		// Раскомментируем
		newLine = strings.Replace(currentLine, lineComment+" ", "", 1)
		newLine = strings.Replace(newLine, lineComment, "", 1)
	} else {
		// Комментируем
		leadingSpaces := len(currentLine) - len(trimmed)
		newLine = currentLine[:leadingSpaces] + lineComment + " " + currentLine[leadingSpaces:]
	}

	hm.app.editor.ReplaceCurrentLine(newLine)
	return true
}

func (hm *HotkeyManager) actionCommentBlock(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Получаем выделенный текст
	selectedText := hm.app.editor.GetSelectedText()
	if selectedText == "" {
		return false
	}

	// Комментируем блок
	language := hm.app.editor.GetLanguage()
	blockStart, blockEnd := getBlockCommentSymbols(language)
	if blockStart == "" || blockEnd == "" {
		// Используем построчное комментирование
		lineComment := getLineCommentSymbol(language)
		if lineComment == "" {
			return false
		}

		lines := strings.Split(selectedText, "\n")
		for i, line := range lines {
			if strings.TrimSpace(line) != "" {
				lines[i] = lineComment + " " + line
			}
		}

		hm.app.editor.ReplaceSelection(strings.Join(lines, "\n"))
	} else {
		// Используем блочное комментирование
		commented := blockStart + " " + selectedText + " " + blockEnd
		hm.app.editor.ReplaceSelection(commented)
	}

	return true
}

// Выделение и курсор
func (hm *HotkeyManager) actionSelectLine(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Выделяем текущую строку
	hm.app.editor.SelectCurrentLine()
	return true
}

func (hm *HotkeyManager) actionSelectWord(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Выделяем слово под курсором
	hm.app.editor.SelectWordAtCursor()
	return true
}

func (hm *HotkeyManager) actionExpandSelection(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Расширяем выделение
	hm.app.editor.ExpandSelection()
	return true
}

func (hm *HotkeyManager) actionShrinkSelection(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Сужаем выделение
	hm.app.editor.ShrinkSelection()
	return true
}

func (hm *HotkeyManager) actionAddCursorAbove(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Добавляем курсор выше
	hm.app.editor.AddCursorAbove()
	return true
}

func (hm *HotkeyManager) actionAddCursorBelow(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Добавляем курсор ниже
	hm.app.editor.AddCursorBelow()
	return true
}

// Фолдинг
func (hm *HotkeyManager) actionFoldBlock(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Сворачиваем текущий блок
	hm.app.editor.FoldCurrentBlock()
	return true
}

func (hm *HotkeyManager) actionUnfoldBlock(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Разворачиваем текущий блок
	hm.app.editor.UnfoldCurrentBlock()
	return true
}

func (hm *HotkeyManager) actionFoldAll(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Сворачиваем все блоки
	hm.app.editor.FoldAll()
	return true
}

func (hm *HotkeyManager) actionUnfoldAll(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Разворачиваем все блоки
	hm.app.editor.UnfoldAll()
	return true
}

// Терминал
func (hm *HotkeyManager) actionOpenTerminal(context HotkeyContext) bool {
	if hm.app == nil || hm.app.terminalMgr == nil {
		return false
	}

	// Открываем терминал по умолчанию
	workingDir := hm.app.getCurrentWorkingDir()
	_, err := hm.app.terminalMgr.OpenTerminal(TerminalDefault, workingDir)
	if err != nil {
		dialog.ShowError(err, hm.window)
		return false
	}

	return true
}

func (hm *HotkeyManager) actionOpenPowerShell(context HotkeyContext) bool {
	if hm.app == nil || hm.app.terminalMgr == nil {
		return false
	}

	// Открываем PowerShell
	workingDir := hm.app.getCurrentWorkingDir()
	err := hm.app.terminalMgr.OpenPowerShell(workingDir)
	if err != nil {
		dialog.ShowError(err, hm.window)
		return false
	}

	return true
}

func (hm *HotkeyManager) actionOpenCMD(context HotkeyContext) bool {
	if hm.app == nil || hm.app.terminalMgr == nil {
		return false
	}

	// Открываем CMD
	workingDir := hm.app.getCurrentWorkingDir()
	err := hm.app.terminalMgr.OpenCMD(workingDir)
	if err != nil {
		dialog.ShowError(err, hm.window)
		return false
	}

	return true
}

func (hm *HotkeyManager) actionCompareFiles(context HotkeyContext) bool {
	if hm.app == nil {
		return false
	}
	hm.app.compareFiles()
	return true
}

// Vim специальные действия
func (hm *HotkeyManager) actionVimEscape(context HotkeyContext) bool {
	hm.vimState.Mode = VimNormal
	if hm.app != nil && hm.app.editor != nil {
		hm.app.editor.SetVimMode(VimNormal)
	}
	return true
}

func (hm *HotkeyManager) actionVimInsert(context HotkeyContext) bool {
	hm.vimState.Mode = VimInsert
	if hm.app != nil && hm.app.editor != nil {
		hm.app.editor.SetVimMode(VimInsert)
	}
	return true
}

func (hm *HotkeyManager) actionVimAppend(context HotkeyContext) bool {
	hm.vimState.Mode = VimInsert
	if hm.app != nil && hm.app.editor != nil {
		// Переместить курсор на одну позицию вправо
		hm.app.editor.MoveCursorRight()
		hm.app.editor.SetVimMode(VimInsert)
	}
	return true
}

func (hm *HotkeyManager) actionVimDeleteLine(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Удаляем текущую строку и сохраняем в регистр
	currentLine := hm.app.editor.GetCurrentLine()
	hm.vimState.Register = currentLine
	hm.app.editor.DeleteCurrentLine()
	return true
}

func (hm *HotkeyManager) actionVimYankLine(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Копируем текущую строку в регистр
	currentLine := hm.app.editor.GetCurrentLine()
	hm.vimState.Register = currentLine
	return true
}

func (hm *HotkeyManager) actionVimPaste(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Вставляем из регистра
	if hm.vimState.Register != "" {
		hm.app.editor.InsertText(hm.vimState.Register)
	}
	return true
}

// Emacs специальные действия
func (hm *HotkeyManager) actionEmacsKillLine(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Удаляем от курсора до конца строки
	killedText := hm.app.editor.KillToEndOfLine()

	// Добавляем в kill ring
	hm.emacsState.KillRing = append(hm.emacsState.KillRing, killedText)
	if len(hm.emacsState.KillRing) > 10 {
		hm.emacsState.KillRing = hm.emacsState.KillRing[1:]
	}
	hm.emacsState.KillRingIndex = len(hm.emacsState.KillRing) - 1

	return true
}

func (hm *HotkeyManager) actionEmacsKillWord(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Удаляем слово
	killedWord := hm.app.editor.KillWord()

	// Добавляем в kill ring
	hm.emacsState.KillRing = append(hm.emacsState.KillRing, killedWord)
	if len(hm.emacsState.KillRing) > 10 {
		hm.emacsState.KillRing = hm.emacsState.KillRing[1:]
	}
	hm.emacsState.KillRingIndex = len(hm.emacsState.KillRing) - 1

	return true
}

func (hm *HotkeyManager) actionEmacsYank(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Вставляем из kill ring
	if len(hm.emacsState.KillRing) > 0 && hm.emacsState.KillRingIndex >= 0 {
		text := hm.emacsState.KillRing[hm.emacsState.KillRingIndex]
		hm.app.editor.InsertText(text)
	}

	return true
}

func (hm *HotkeyManager) actionEmacsSetMark(context HotkeyContext) bool {
	if hm.app == nil || hm.app.editor == nil {
		return false
	}

	// Устанавливаем метку в текущей позиции
	position := hm.app.editor.GetCursorPosition()
	hm.emacsState.Mark = &position
	hm.emacsState.MarkActive = true

	return true
}

// getActionDescription возвращает описание действия
func (hm *HotkeyManager) getActionDescription(actionID string) string {
	descriptions := map[string]string{
		"new_file":         "Create a new file",
		"open_file":        "Open an existing file",
		"save_file":        "Save the current file",
		"save_as_file":     "Save the current file with a new name",
		"close_file":       "Close the current file",
		"close_all":        "Close all open files",
		"cut":              "Cut selected text",
		"copy":             "Copy selected text",
		"paste":            "Paste text from clipboard",
		"select_all":       "Select all text",
		"undo":             "Undo last action",
		"redo":             "Redo last undone action",
		"find":             "Find text",
		"find_next":        "Find next occurrence",
		"find_previous":    "Find previous occurrence",
		"replace":          "Replace text",
		"find_in_files":    "Find text in multiple files",
		"go_to_line":       "Go to specific line number",
		"go_to_symbol":     "Go to symbol definition",
		"go_to_definition": "Go to symbol definition",
		"toggle_sidebar":   "Show/hide sidebar",
		"toggle_minimap":   "Show/hide minimap",
		"toggle_terminal":  "Show/hide terminal",
		"command_palette":  "Open command palette",
		"file_explorer":    "Open file explorer",
		"file_switcher":    "Switch between recent files",
	}

	if desc, exists := descriptions[actionID]; exists {
		return desc
	}

	return "Custom action"
}

// Вспомогательные функции

func getLineCommentSymbol(language string) string {
	commentSymbols := map[string]string{
		"go":         "//",
		"java":       "//",
		"c":          "//",
		"cpp":        "//",
		"javascript": "//",
		"typescript": "//",
		"rust":       "//",
		"python":     "#",
		"ruby":       "#",
		"shell":      "#",
		"yaml":       "#",
		"toml":       "#",
		"sql":        "--",
		"lua":        "--",
		"haskell":    "--",
	}

	if symbol, ok := commentSymbols[language]; ok {
		return symbol
	}
	return "//"
}

func getBlockCommentSymbols(language string) (string, string) {
	blockComments := map[string][2]string{
		"go":         {"/*", "*/"},
		"java":       {"/*", "*/"},
		"c":          {"/*", "*/"},
		"cpp":        {"/*", "*/"},
		"javascript": {"/*", "*/"},
		"typescript": {"/*", "*/"},
		"rust":       {"/*", "*/"},
		"css":        {"/*", "*/"},
		"html":       {"<!--", "-->"},
		"xml":        {"<!--", "-->"},
		"sql":        {"/*", "*/"},
		"lua":        {"--[[", "]]"},
	}

	if symbols, ok := blockComments[language]; ok {
		return symbols[0], symbols[1]
	}
	return "", ""
}

// getContextPriority возвращает приоритет контекста
func (hm *HotkeyManager) getContextPriority(context HotkeyContext) int {
	priorities := map[HotkeyContext]int{
		ContextGlobal:         0,
		ContextEditor:         10,
		ContextSidebar:        5,
		ContextMinimap:        5,
		ContextDialog:         20,
		ContextTerminal:       15,
		ContextSearch:         15,
		ContextMenu:           25,
		ContextCommandPalette: 30,
	}

	if priority, exists := priorities[context]; exists {
		return priority
	}

	return 0
}

// contextToString преобразует контекст в строку
func (hm *HotkeyManager) contextToString(context HotkeyContext) string {
	names := map[HotkeyContext]string{
		ContextGlobal:         "Global",
		ContextEditor:         "Editor",
		ContextSidebar:        "Sidebar",
		ContextMinimap:        "Minimap",
		ContextDialog:         "Dialog",
		ContextTerminal:       "Terminal",
		ContextSearch:         "Search",
		ContextMenu:           "Menu",
		ContextCommandPalette: "Command Palette",
	}

	if name, exists := names[context]; exists {
		return name
	}

	return "Unknown"
}

// loadVimBindings загружает специальные Vim привязки
func (hm *HotkeyManager) loadVimBindings() {
	bindings := []struct {
		id     string
		combo  string
		action string
	}{
		{id: "vim_escape", combo: "Escape", action: "vim_escape"},
		{id: "vim_escape_ctrl", combo: "Ctrl+[", action: "vim_escape"},
		{id: "vim_insert", combo: "Alt+I", action: "vim_insert"},
		{id: "vim_append", combo: "Alt+A", action: "vim_append"},
		{id: "vim_delete_line", combo: "Alt+D", action: "vim_delete_line"},
		{id: "vim_yank_line", combo: "Alt+Y", action: "vim_yank_line"},
		{id: "vim_paste", combo: "Alt+P", action: "vim_paste"},
	}

	for _, b := range bindings {
		hm.registerShortcut(b.id, b.combo, b.action, ContextEditor, "Vim")
		if sc, ok := hm.shortcuts[b.id]; ok {
			sc.RequiredMode = ModeVim
		}
	}
}

// loadEmacsBindings загружает специальные Emacs привязки
func (hm *HotkeyManager) loadEmacsBindings() {
	bindings := []struct {
		id     string
		combo  string
		action string
	}{
		{id: "emacs_kill_line", combo: "Ctrl+K", action: "emacs_kill_line"},
		{id: "emacs_kill_word", combo: "Alt+D", action: "emacs_kill_word"},
		{id: "emacs_yank", combo: "Ctrl+Y", action: "emacs_yank"},
		{id: "emacs_set_mark", combo: "Ctrl+Space", action: "emacs_set_mark"},
	}

	for _, b := range bindings {
		hm.registerShortcut(b.id, b.combo, b.action, ContextEditor, "Emacs")
		if sc, ok := hm.shortcuts[b.id]; ok {
			sc.RequiredMode = ModeEmacs
		}
	}
}

// ConflictResolver implementation

// NewConflictResolver создает новый резолвер конфликтов
func NewConflictResolver() *ConflictResolver {
	return &ConflictResolver{
		rules: []ConflictRule{},
		priorities: map[HotkeyContext]int{
			ContextGlobal:         0,
			ContextEditor:         10,
			ContextSidebar:        5,
			ContextMinimap:        5,
			ContextDialog:         20,
			ContextTerminal:       15,
			ContextSearch:         15,
			ContextMenu:           25,
			ContextCommandPalette: 30,
		},
		overrides: make(map[string]string),
	}
}

// resolve разрешает конфликт горячих клавиш
func (cr *ConflictResolver) resolve(newShortcut *RegisteredShortcut, conflicts []*RegisteredShortcut) ConflictResolution {
	// Простая реализация - приоритет по контексту
	return ResolutionHighestPriority
}

// KeyLogger implementation

// logKey записывает нажатие клавиши в лог
func (kl *KeyLogger) logKey(key fyne.KeyName, modifiers ModifierState, context HotkeyContext, mode InputMode) {
	if !kl.enabled {
		return
	}

	kl.mutex.Lock()
	defer kl.mutex.Unlock()

	entry := KeyLogEntry{
		Timestamp: time.Now(),
		Key:       key,
		Modifiers: modifiers,
		Context:   context,
		Mode:      mode,
		Action:    "key_press",
		Result:    "logged",
	}

	kl.entries = append(kl.entries, entry)

	// Ограничиваем размер лога
	if len(kl.entries) > kl.maxEntries {
		kl.entries = kl.entries[1:]
	}
}

// logAction записывает выполнение действия в лог
func (kl *KeyLogger) logAction(actionID string, result bool) {
	if !kl.enabled {
		return
	}

	kl.mutex.Lock()
	defer kl.mutex.Unlock()

	resultStr := "success"
	if !result {
		resultStr = "failed"
	}

	entry := KeyLogEntry{
		Timestamp: time.Now(),
		Action:    actionID,
		Result:    resultStr,
	}

	kl.entries = append(kl.entries, entry)

	if len(kl.entries) > kl.maxEntries {
		kl.entries = kl.entries[1:]
	}
}

// Cleanup очищает ресурсы менеджера горячих клавиш
func (hm *HotkeyManager) Cleanup() {
	hm.clearPendingKeys()

	// Удаляем все shortcuts из Fyne
	for _, shortcut := range hm.shortcuts {
		if shortcut.Context == ContextGlobal || shortcut.Context == ContextEditor {
			hm.window.Canvas().RemoveShortcut(shortcut.Shortcut)
		}
	}
}
