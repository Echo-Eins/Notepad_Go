package main

import (
	"context"
	"errors"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/fsnotify/fsnotify"
	"image/color"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

// EditorWidget - основной виджет редактора с полным функционалом
type EditorWidget struct {
	widget.BaseWidget

	// Основные компоненты
	content         *widget.Entry    // Изменено с RichText на Entry для редактирования
	richContent     *widget.RichText // Для отображения с подсветкой
	lineNumbers     *widget.Label
	scrollContainer *container.Scroll
	mainContainer   *fyne.Container // Изменено с container.Border на *fyne.Container

	// Конфигурация
	config *Config
	colors EditorColors

	// Состояние редактора
	filePath    string
	textContent string
	isDirty     bool
	isReadOnly  bool
	encoding    string
	lineEnding  string
	language    string

	// Позиция курсора
	cursorRow      int
	cursorCol      int
	selectionStart TextPosition
	selectionEnd   TextPosition

	// История изменений
	undoStack     []EditorCommand
	redoStack     []EditorCommand
	maxUndoLevels int

	// Подсветка синтаксиса
	lexer        chroma.Lexer
	style        *chroma.Style
	formatter    chroma.Formatter
	syntaxTokens []chroma.Token
	syntaxCache  map[string][]chroma.Token

	// Фолдинг и сворачивание
	foldedRanges     map[int]FoldRange
	foldingSupported bool

	// Bracket matching
	matchingBrackets map[int]int
	bracketPairs     []BracketPair

	// Автосохранение
	autoSaveTimer *time.Timer
	lastSavedHash string
	lastModified  time.Time

	// File watching
	fileWatcher    *fsnotify.Watcher
	watcherCancel  context.CancelFunc
	searchResults  []TextRange
	highlightTimer *time.Timer

	// Callbacks
	onContentChanged func(content string)
	onCursorChanged  func(row, col int)
	onFileChanged    func(filepath string)

	// Мультикурсоры
	cursors         []TextPosition
	mainCursorIndex int

	// Синхронизация
	renderMutex sync.Mutex

	// Clickable ranges (для ссылок, импортов и т.д.)
	clickableRanges []ClickableRange

	// Автодополнение
	autoCompleteActive bool
	autoCompleteList   []CompletionItem
	autoCompleteWidget *widget.PopUp

	// Indent guides
	indentGuides    []IndentGuide
	indentContainer *fyne.Container
	fileName        string

	vimMode VimMode

	// Bookmarks
	bookmarks map[int]Bookmark

	// Lint errors by line number
	lintLines map[int]string

	// Bookmark callbacks
	onBookmarksChanged func()
}

// Bookmark represents a bookmark in editor
type Bookmark struct {
	Line int
	Name string
}

// IsDirty возвращает true, если есть несохраненные изменения
func (e *EditorWidget) IsDirty() bool {
	return e.isDirty
}

// ErrFileTooLarge is returned when a file exceeds the configured maximum size
var ErrFileTooLarge = errors.New("file exceeds maximum allowed size")

// LoadFile загружает файл в редактор
func (e *EditorWidget) LoadFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if e.config != nil && e.config.Editor.MaxFileSize > 0 && info.Size() > e.config.Editor.MaxFileSize {
		dialog.ShowInformation("File Too Large", fmt.Sprintf("File size (%d bytes) exceeds limit (%d bytes)", info.Size(), e.config.Editor.MaxFileSize), fyne.CurrentApp().Driver().AllWindows()[0])
		return ErrFileTooLarge
	}
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	e.filePath = path
	e.fileName = filepath.Base(path)
	e.textContent = string(content)
	e.content.SetText(e.textContent)
	e.isDirty = false
	e.lastModified = info.ModTime()
	e.detectLanguage()
	e.updateDisplay()
	e.ClearLintErrors()
	e.startFileWatcher()

	return nil
}

// SaveFile сохраняет содержимое в файл
func (e *EditorWidget) SaveFile() error {
	if e.filePath == "" {
		return fmt.Errorf("no file path specified")
	}

	content := e.GetFullText()
	err := ioutil.WriteFile(e.filePath, []byte(content), 0644)
	if err != nil {
		return err
	}

	e.isDirty = false
	if info, err := os.Stat(e.filePath); err == nil {
		e.lastModified = info.ModTime()
	}
	return nil
}

// SaveAsFile сохраняет файл с новым именем
func (e *EditorWidget) SaveAsFile(path string) error {
	e.filePath = path
	e.fileName = filepath.Base(path)
	return e.SaveFile()
}

// SetContent устанавливает содержимое редактора
func (e *EditorWidget) SetContent(content string) {
	e.textContent = content
	e.content.SetText(content)
	e.isDirty = true
	e.updateDisplay()
}

// AddBookmark adds a bookmark at specific line
func (e *EditorWidget) AddBookmark(line int, name string) {
	if line < 1 {
		return
	}
	if e.bookmarks == nil {
		e.bookmarks = make(map[int]Bookmark)
	}
	e.bookmarks[line] = Bookmark{Line: line, Name: name}
	e.updateLineNumbers()
	if e.onBookmarksChanged != nil {
		e.onBookmarksChanged()
	}
}

// RemoveBookmark deletes bookmark from line
func (e *EditorWidget) RemoveBookmark(line int) {
	if e.bookmarks == nil {
		return
	}
	delete(e.bookmarks, line)
	e.updateLineNumbers()
	if e.onBookmarksChanged != nil {
		e.onBookmarksChanged()
	}
}

// IsLineBookmarked checks if line has bookmark
func (e *EditorWidget) IsLineBookmarked(line int) bool {
	if e.bookmarks == nil {
		return false
	}
	_, ok := e.bookmarks[line]
	return ok
}

// GetBookmarks returns all bookmarks
func (e *EditorWidget) GetBookmarks() []Bookmark {
	result := []Bookmark{}
	for _, b := range e.bookmarks {
		result = append(result, b)
	}
	return result
}

// GoToBookmark moves cursor to bookmark line
func (e *EditorWidget) GoToBookmark(line int) error {
	cmd := &GoToLineCommand{lineNumber: line}
	return cmd.Execute(e)
}

// SetLintErrors highlights line numbers that contain linter errors
func (e *EditorWidget) SetLintErrors(errors []CompilerError) {
	if len(errors) == 0 {
		e.lintLines = nil
		e.updateLineNumbers()
		return
	}

	if e.lintLines == nil {
		e.lintLines = make(map[int]string)
	} else {
		for k := range e.lintLines {
			delete(e.lintLines, k)
		}
	}

	for _, err := range errors {
		if err.Line > 0 {
			e.lintLines[err.Line] = err.Message
		}
	}
	e.updateLineNumbers()
}

// ClearLintErrors removes all linter error highlights
func (e *EditorWidget) ClearLintErrors() {
	if e.lintLines != nil {
		e.lintLines = nil
		e.updateLineNumbers()
	}
}

// getLineCount возвращает количество строк
func (e *EditorWidget) getLineCount() int {
	if e.textContent == "" {
		return 1
	}
	return len(strings.Split(e.textContent, "\n"))
}

// updateLineNumbers пересчитывает и обновляет виджет номеров строк
func (e *EditorWidget) updateLineNumbers() {
	if e.lineNumbers == nil {
		return
	}

	lines := e.getLineCount()
	digits := len(fmt.Sprintf("%d", lines))
	var b strings.Builder
	for i := 1; i <= lines; i++ {
		if i > 1 {
			b.WriteRune('\n')
		}
		marker := "  "
		if e.IsLineBookmarked(i) {
			marker = "★ "
		}
		b.WriteString(fmt.Sprintf("%s%*d", marker, digits, i))
	}

	e.lineNumbers.SetText(b.String())

	e.lineNumbers.Refresh()
}

// setupSyntaxHighlighter настраивает подсветку синтаксиса
func (e *EditorWidget) setupSyntaxHighlighter() {
	// Инициализируем стиль подсветки
	e.style = styles.Get("monokai")
	if e.style == nil {
		e.style = styles.Fallback
	}

	// Создаем форматтер
	e.formatter = formatters.Get("terminal")
	if e.formatter == nil {
		e.formatter = formatters.Fallback
	}
}

// CompletionItem represents a single autocomplete suggestion
type CompletionItem struct {
	Text        string
	Description string
}

// TextPosition представляет позицию в тексте
type TextPosition struct {
	Row int
	Col int
}

// TextRange представляет диапазон текста
type TextRange struct {
	Start TextPosition
	End   TextPosition
	Text  string
}

// ClickableRange представляет кликабельный элемент
type ClickableRange struct {
	Range   TextRange
	Type    ClickableType
	Target  string
	Tooltip string
	OnClick func()
	OnHover func()
}

// ClickableType тип кликабельного элемента
type ClickableType int

const (
	ClickableImport ClickableType = iota
	ClickableFunction
	ClickableVariable
	ClickableURL
	ClickableFile
	ClickableCommand
	ClickableColor
	ClickableClass
	ClickableTODO
)

// FoldRange представляет свернутый блок кода
type FoldRange struct {
	Start       int // Start is the first line of the folded range
	End         int // End is the last line of the folded range
	IsFolded    bool
	IndentLevel int
	Label       string
	Lines       []string
}

// IndentGuide представляет направляющую отступа
type IndentGuide struct {
	Row        int
	Col        int
	Level      int
	IsVertical bool
	Label      string
	OnClick    func()
}

// BracketPair представляет пару скобок
type BracketPair struct {
	Open      TextPosition
	Close     TextPosition
	Type      rune
	IsMatched bool
}

// NewEditor создает новый экземпляр редактора
func NewEditor(config *Config) *EditorWidget {
	editor := &EditorWidget{
		config:           config,
		colors:           GetEditorColors(config.App.Theme == "dark"), // Исправлено: config.App.Theme
		foldedRanges:     make(map[int]FoldRange),
		matchingBrackets: make(map[int]int),
		syntaxCache:      make(map[string][]chroma.Token),
		encoding:         "UTF-8",
		lineEnding:       "\r\n", // Windows default
		maxUndoLevels:    100,
		cursors:          []TextPosition{},
	}

	editor.ExtendBaseWidget(editor)
	editor.setupComponents()
	editor.setupSyntaxHighlighter()
	editor.setupAutoSave()
	editor.bindEvents()

	return editor
}

// setupComponents создает и настраивает UI компоненты
func (e *EditorWidget) setupComponents() {
	// Создаем основной текстовый виджет (Entry для редактирования)
	e.content = widget.NewMultiLineEntry()
	e.content.Wrapping = fyne.TextWrapWord

	// Настраиваем поведение в зависимости от WordWrap
	if e.config.Editor.WordWrap { // Исправлено: config.Editor.WordWrap
		e.content.Wrapping = fyne.TextWrapWord
	} else {
		e.content.Wrapping = fyne.TextWrapOff
	}

	// Создаем RichText для отображения с подсветкой
	e.richContent = widget.NewRichText()

	// Создаем виджет номеров строк
	e.lineNumbers = widget.NewLabel("")
	e.lineNumbers.Wrapping = fyne.TextWrapOff
	e.lineNumbers.Alignment = fyne.TextAlignTrailing
	e.lineNumbers.TextStyle = fyne.TextStyle{Monospace: true}
	e.updateLineNumbers()

	// Контейнер для направляющих отступа
	e.indentContainer = container.NewWithoutLayout()

	// Создаем контейнер с прокруткой
	editorLayer := container.NewMax(e.richContent, e.indentContainer, e.content)
	var editorContent fyne.CanvasObject
	if e.config.Editor.ShowLineNumbers {
		editorContent = container.NewBorder(nil, nil, container.NewVBox(e.lineNumbers), nil, editorLayer)
	} else {
		editorContent = editorLayer
	}

	e.scrollContainer = container.NewScroll(editorContent)
	e.scrollContainer.SetMinSize(fyne.NewSize(800, 600))

	// Основной контейнер
	e.mainContainer = container.NewMax(e.scrollContainer) // Используем container.NewMax вместо Border
}

// setupAutoSave настраивает автосохранение
func (e *EditorWidget) setupAutoSave() {
	if !e.config.Editor.AutoSave { // Исправлено: config.Editor.AutoSave
		return
	}

	// AutoSaveDelay в секундах, конвертируем в Duration
	duration := time.Duration(e.config.Editor.AutoSaveDelay) * time.Second // Исправлено: AutoSaveDelay вместо AutoSaveMinutes
	e.autoSaveTimer = time.AfterFunc(duration, func() {
		if e.isDirty && e.filePath != "" {
			e.SaveFile()
			e.resetAutoSaveTimer()
		}
	})
}

func (e *EditorWidget) bindEvents() {
	// Обработчик изменения текста для Entry
	e.content.OnChanged = func(text string) {
		e.textContent = text
		e.onTextChanged()
	}

	// Обработчик изменения позиции курсора
	e.content.OnCursorChanged = func() {
		// Обновляем позицию курсора
		e.updateCursorPosition()
	}

	// Добавляем обработку кликов через расширение базового виджета
	e.ExtendBaseWidget(e)
}

// FocusGained is called when the editor receives focus.
// Forward the event to the underlying Entry widget so that
// text input continues to work as expected.
func (e *EditorWidget) FocusGained() {
	if e.content != nil {
		e.content.FocusGained()
	}
}

// FocusLost is called when the editor loses focus.
// We forward this event to the Entry widget to ensure
// it updates its state appropriately.
func (e *EditorWidget) FocusLost() {
	if e.content != nil {
		e.content.FocusLost()
	}
}

// TypedRune forwards rune input to the underlying Entry widget.
func (e *EditorWidget) TypedRune(r rune) {
	if e.content != nil {
		e.content.TypedRune(r)
	}
}

// TypedKey forwards key events to the underlying Entry widget with auto-indentation support.
func (e *EditorWidget) TypedKey(event *fyne.KeyEvent) {
	if e.config != nil && e.config.Editor.AutoIndent && event.Name == fyne.KeyReturn {
		e.handleAutoIndent()
		return
	}
	if e.content != nil {
		e.content.TypedKey(event)
	}
}

// handleAutoIndent inserts indentation on newline based on current line context.
func (e *EditorWidget) handleAutoIndent() {
	line := e.GetCurrentLine()
	indent := e.getIndentSpaces(line)
	trimmed := strings.TrimSpace(line)
	if strings.HasSuffix(trimmed, "{") {
		indent += 4
	}
	if strings.HasPrefix(trimmed, "}") && indent >= 4 {
		indent -= 4
	}
	e.content.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	for i := 0; i < indent; i++ {
		e.content.TypedRune(' ')
	}
}

// getIndentSpaces counts leading spaces in a line.
func (e *EditorWidget) getIndentSpaces(line string) int {
	count := 0
	for _, r := range line {
		if r == ' ' {
			count++
		} else if r == '\t' {
			count += 4
		} else {
			break
		}
	}
	return count
}

// TappedSecondary обрабатывает правый клик мыши
func (e *EditorWidget) TappedSecondary(event *fyne.PointEvent) {
	e.showContextMenu(event.Position)
}

// updateCursorPosition обновляет внутреннее состояние позиции курсора
func (e *EditorWidget) updateCursorPosition() {
	e.cursorRow = e.content.CursorRow
	e.cursorCol = e.content.CursorColumn
	if e.onCursorChanged != nil {
		e.onCursorChanged(e.cursorRow, e.cursorCol)
	}
	e.highlightWordAtCursor()
	e.updateLineNumbers()
}

// getWordAtCursor возвращает слово под текущим курсором
func (e *EditorWidget) getWordAtCursor() string {
	lines := strings.Split(e.textContent, "\n")
	if e.cursorRow < 0 || e.cursorRow >= len(lines) {
		return ""
	}
	runes := []rune(lines[e.cursorRow])
	if len(runes) == 0 {
		return ""
	}
	col := e.cursorCol
	if col >= len(runes) {
		col = len(runes) - 1
	}
	if col < 0 || !isWordRune(runes[col]) {
		return ""
	}
	start, end := col, col
	for start > 0 && isWordRune(runes[start-1]) {
		start--
	}
	for end < len(runes) && isWordRune(runes[end]) {
		end++
	}
	return string(runes[start:end])
}

// isWordRune проверяет, является ли символ частью слова
func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// highlightWordAtCursor ищет и подсвечивает совпадения текущего слова
func (e *EditorWidget) highlightWordAtCursor() {
	if e.config == nil || !e.config.Editor.HighlightCurrentWord {
		return
	}

	word := e.getWordAtCursor()
	if word == "" {
		e.searchResults = nil
		e.applyTokensToRichText()
		return
	}

	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(word) + `\b`)
	matches := re.FindAllStringIndex(e.textContent, -1)

	results := make([]TextRange, 0, len(matches))
	for _, m := range matches {
		start := e.indexToPosition(m[0])
		end := e.indexToPosition(m[1])
		results = append(results, TextRange{Start: start, End: end, Text: word})
	}
	e.searchResults = results
	e.applyTokensToRichText()

	if e.highlightTimer != nil {
		e.highlightTimer.Stop()
	}
	e.highlightTimer = time.AfterFunc(2*time.Second, func() {
		e.searchResults = nil
		e.applyTokensToRichText()
	})
}

// indexToPosition преобразует индекс в позицию текста
func (e *EditorWidget) indexToPosition(idx int) TextPosition {
	if idx < 0 {
		return TextPosition{}
	}
	lines := strings.Split(e.textContent, "\n")
	remaining := idx
	for row, line := range lines {
		if remaining <= len(line) {
			return TextPosition{Row: row, Col: remaining}
		}
		remaining -= len(line) + 1
	}
	if len(lines) == 0 {
		return TextPosition{}
	}
	lastRow := len(lines) - 1
	return TextPosition{Row: lastRow, Col: len(lines[lastRow])}
}

// isIndexInSearchResults проверяет, пересекается ли диапазон с найденными совпадениями
func (e *EditorWidget) isIndexInSearchResults(start, end int) bool {
	for _, r := range e.searchResults {
		rs := e.positionToIndex(r.Start)
		re := e.positionToIndex(r.End)
		if start < re && end > rs {
			return true
		}
	}
	return false
}

// resetAutoSaveTimer сбрасывает таймер автосохранения
func (e *EditorWidget) resetAutoSaveTimer() {
	if e.autoSaveTimer != nil {
		e.autoSaveTimer.Stop()
	}

	if e.config.Editor.AutoSave {
		duration := time.Duration(e.config.Editor.AutoSaveDelay) * time.Second
		e.autoSaveTimer = time.AfterFunc(duration, func() {
			if e.isDirty && e.filePath != "" {
				e.SaveFile()
				e.resetAutoSaveTimer()
			}
		})
	}
}

// detectLanguage определяет язык программирования по расширению
func (e *EditorWidget) detectLanguage() {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(e.filePath), "."))

	var lexerName string
	switch ext {
	case "go":
		lexerName = "go"
	case "rs":
		lexerName = "rust"
	case "py":
		lexerName = "python"
	case "c", "h":
		lexerName = "c"
	case "java":
		lexerName = "java"
	case "js":
		lexerName = "javascript"
	case "ts":
		lexerName = "typescript"
	case "html":
		lexerName = "html"
	case "css":
		lexerName = "css"
	case "json":
		lexerName = "json"
	case "xml":
		lexerName = "xml"
	case "md":
		lexerName = "markdown"
	default:
		lexerName = "text"
	}
	e.language = lexerName
	e.lexer = lexers.Get(lexerName)
	if e.lexer == nil {
		e.lexer = lexers.Fallback
	}
}

// updateDisplay обновляет отображение редактора
func (e *EditorWidget) updateDisplay() {
	e.renderMutex.Lock()
	defer e.renderMutex.Unlock()

	// Применяем подсветку синтаксиса
	e.applySyntaxHighlighting()

	// Обновляем bracket matching
	e.updateBracketMatching()

	// Обновляем фолдинг
	e.updateCodeFolding()

	// Обновляем отступы
	e.updateIndentGuides()

	// Обновляем номера строк и содержимое в главном потоке UI
	fyne.Do(func() {
		if e.lineNumbers != nil {
			e.updateLineNumbers()
		}
		e.content.Refresh()
	})
}

// Дополнительные методы для поддержки HotkeyManager

// GetSelectedText возвращает выделенный текст
func (e *EditorWidget) GetSelectedText() string {
	// Если используем Entry, можем получить выделение
	if e.content.SelectedText() != "" {
		return e.content.SelectedText()
	}
	return ""
}

// GetCurrentLine возвращает текущую строку
func (e *EditorWidget) GetCurrentLine() string {
	lines := strings.Split(e.textContent, "\n")
	if e.cursorRow >= 0 && e.cursorRow < len(lines) {
		return lines[e.cursorRow]
	}
	return ""
}

// SelectAll выделяет весь текст
func (e *EditorWidget) SelectAll() {
	e.content.TypedShortcut(&fyne.ShortcutSelectAll{})
}

// GetCursorPosition возвращает позицию курсора
func (e *EditorWidget) GetCursorPosition() TextPosition {
	return TextPosition{
		Row: e.cursorRow,
		Col: e.cursorCol,
	}
}

// Clear очищает содержимое редактора
func (e *EditorWidget) Clear() {
	e.textContent = ""
	e.content.SetText("")
	e.cursorRow = 0
	e.cursorCol = 0
	e.selectionStart = TextPosition{}
	e.selectionEnd = TextPosition{}
	e.isDirty = false
	e.filePath = ""
	e.fileName = ""
	e.updateDisplay()
}

// SetFilePath устанавливает путь к текущему файлу
func (e *EditorWidget) SetFilePath(path string) {
	e.filePath = path
	e.fileName = filepath.Base(path)
}

// GetLanguage возвращает определенный для текущего файла язык
func (e *EditorWidget) GetLanguage() string {
	return e.language
}

// ExecuteCommand выполняет команду с добавлением в историю
func (e *EditorWidget) ExecuteCommand(cmd EditorCommand) error {
	if err := cmd.Execute(e); err != nil {
		return err
	}
	e.undoStack = append(e.undoStack, cmd)
	e.redoStack = nil
	if len(e.undoStack) > e.maxUndoLevels {
		e.undoStack = e.undoStack[1:]
	}
	e.isDirty = true
	return nil
}

// Undo отменяет последнюю команду
func (e *EditorWidget) Undo() {
	if len(e.undoStack) == 0 {
		return
	}
	cmd := e.undoStack[len(e.undoStack)-1]
	e.undoStack = e.undoStack[:len(e.undoStack)-1]
	if err := cmd.Undo(e); err == nil {
		e.redoStack = append(e.redoStack, cmd)
	}
}

// Redo повторяет отмененную команду
func (e *EditorWidget) Redo() {
	if len(e.redoStack) == 0 {
		return
	}
	cmd := e.redoStack[len(e.redoStack)-1]
	e.redoStack = e.redoStack[:len(e.redoStack)-1]
	if err := cmd.Execute(e); err == nil {
		e.undoStack = append(e.undoStack, cmd)
	}
}

// ReplaceSelection заменяет выделенный текст новым
func (e *EditorWidget) ReplaceSelection(newText string) {
	sel := e.content.SelectedText()
	if sel == "" {
		return
	}
	start := e.selectionStart
	end := e.selectionEnd
	if start.Row > end.Row || (start.Row == end.Row && start.Col > end.Col) {
		start, end = end, start
	}
	lines := strings.Split(e.textContent, "\n")
	if start.Row == end.Row {
		line := lines[start.Row]
		lines[start.Row] = line[:start.Col] + newText + line[end.Col:]
	} else {
		first := lines[start.Row][:start.Col]
		last := lines[end.Row][end.Col:]
		lines = append(lines[:start.Row], append([]string{first + newText + last}, lines[end.Row+1:]...)...)
	}
	e.textContent = strings.Join(lines, "\n")
	e.content.SetText(e.textContent)
	e.cursorRow = start.Row
	e.cursorCol = start.Col + len(newText)
	e.isDirty = true
	e.updateDisplay()
}

// ReplaceCurrentLine заменяет текущую строку
func (e *EditorWidget) ReplaceCurrentLine(newLine string) {
	lines := strings.Split(e.textContent, "\n")
	if e.cursorRow >= 0 && e.cursorRow < len(lines) {
		lines[e.cursorRow] = newLine
		e.textContent = strings.Join(lines, "\n")
		e.content.SetText(e.textContent)
		e.isDirty = true
		e.updateDisplay()
	}
}

// SelectWordAtCursor выделяет слово под курсором
func (e *EditorWidget) SelectWordAtCursor() {
	word := e.GetWordAtCursor()
	if word == "" {
		return
	}
	line := e.GetCurrentLine()
	idx := strings.Index(line, word)
	if idx >= 0 {
		e.selectionStart = TextPosition{Row: e.cursorRow, Col: idx}
		e.selectionEnd = TextPosition{Row: e.cursorRow, Col: idx + len(word)}
		e.cursorCol = idx + len(word)
		e.content.CursorColumn = e.cursorCol
	}
}

// SelectCurrentLine выделяет текущую строку целиком
func (e *EditorWidget) SelectCurrentLine() {
	lines := strings.Split(e.textContent, "\n")
	if e.cursorRow < 0 || e.cursorRow >= len(lines) {
		return
	}
	line := lines[e.cursorRow]
	e.selectionStart = TextPosition{Row: e.cursorRow, Col: 0}
	e.selectionEnd = TextPosition{Row: e.cursorRow, Col: len(line)}
	e.cursorCol = len(line)
	e.content.CursorColumn = e.cursorCol
}

// ExpandSelection расширяет выделение до слова
func (e *EditorWidget) ExpandSelection() {
	e.SelectWordAtCursor()
}

// ShrinkSelection снимает выделение
func (e *EditorWidget) ShrinkSelection() {
	e.selectionStart = TextPosition{}
	e.selectionEnd = TextPosition{}
	e.content.SetText(e.textContent)
}

// AddCursorAbove добавляет курсор выше текущего
func (e *EditorWidget) AddCursorAbove() {
	if e.cursorRow > 0 {
		pos := TextPosition{Row: e.cursorRow - 1, Col: e.cursorCol}
		e.cursors = append(e.cursors, pos)
	}
}

// AddCursorBelow добавляет курсор ниже текущего
func (e *EditorWidget) AddCursorBelow() {
	lines := strings.Split(e.textContent, "\n")
	if e.cursorRow < len(lines)-1 {
		pos := TextPosition{Row: e.cursorRow + 1, Col: e.cursorCol}
		e.cursors = append(e.cursors, pos)
	}
}

// MoveCursorRight перемещает курсор вправо с переходом на следующую строку
func (e *EditorWidget) MoveCursorRight() {
	lines := strings.Split(e.textContent, "\n")
	if e.cursorRow < 0 || e.cursorRow >= len(lines) {
		return
	}
	line := lines[e.cursorRow]
	if e.cursorCol < len(line) {
		e.cursorCol++
	} else if e.cursorRow < len(lines)-1 {
		e.cursorRow++
		e.cursorCol = 0
	}
	e.content.CursorRow = e.cursorRow
	e.content.CursorColumn = e.cursorCol
	if e.onCursorChanged != nil {
		e.onCursorChanged(e.cursorRow, e.cursorCol)
	}
}

// DeleteCurrentLine удаляет текущую строку
func (e *EditorWidget) DeleteCurrentLine() {
	lines := strings.Split(e.textContent, "\n")
	if e.cursorRow >= 0 && e.cursorRow < len(lines) {
		lines = append(lines[:e.cursorRow], lines[e.cursorRow+1:]...)
		if e.cursorRow >= len(lines) {
			e.cursorRow = len(lines) - 1
		}
		e.cursorCol = 0
		e.textContent = strings.Join(lines, "\n")
		e.content.SetText(e.textContent)
		e.isDirty = true
		e.updateDisplay()
	}
}

// InsertText вставляет текст в текущую позицию курсора
func (e *EditorWidget) InsertText(text string) {
	lines := strings.Split(e.textContent, "\n")
	if e.cursorRow >= 0 && e.cursorRow < len(lines) {
		line := lines[e.cursorRow]
		if e.cursorCol > len(line) {
			e.cursorCol = len(line)
		}
		newLine := line[:e.cursorCol] + text + line[e.cursorCol:]
		lines[e.cursorRow] = newLine
		e.cursorCol += len(text)
		e.textContent = strings.Join(lines, "\n")
		e.content.SetText(e.textContent)
		e.isDirty = true
		e.updateDisplay()
	}
}

// KillToEndOfLine удаляет текст от курсора до конца строки
func (e *EditorWidget) KillToEndOfLine() string {
	lines := strings.Split(e.textContent, "\n")
	if e.cursorRow >= 0 && e.cursorRow < len(lines) {
		line := lines[e.cursorRow]
		if e.cursorCol >= len(line) {
			return ""
		}
		killed := line[e.cursorCol:]
		lines[e.cursorRow] = line[:e.cursorCol]
		e.textContent = strings.Join(lines, "\n")
		e.content.SetText(e.textContent)
		e.isDirty = true
		e.updateDisplay()
		return killed
	}
	return ""
}

// KillWord удаляет слово справа от курсора
func (e *EditorWidget) KillWord() string {
	lines := strings.Split(e.textContent, "\n")
	if e.cursorRow >= 0 && e.cursorRow < len(lines) {
		line := lines[e.cursorRow]
		if e.cursorCol >= len(line) {
			return ""
		}
		rest := line[e.cursorCol:]
		re := regexp.MustCompile(`^\w+`)
		loc := re.FindStringIndex(rest)
		if loc == nil {
			return ""
		}
		killed := rest[:loc[1]]
		lines[e.cursorRow] = line[:e.cursorCol] + rest[loc[1]:]
		e.textContent = strings.Join(lines, "\n")
		e.content.SetText(e.textContent)
		e.isDirty = true
		e.updateDisplay()
		return killed
	}
	return ""
}

// FindNext ищет следующее вхождение строки
func (e *EditorWidget) FindNext(term string) bool {
	if term == "" {
		return false
	}
	content := e.textContent
	startIdx := e.cursorIndex()
	idx := strings.Index(content[startIdx:], term)
	if idx == -1 {
		idx = strings.Index(content, term)
		if idx == -1 {
			return false
		}
	} else {
		idx += startIdx
	}
	e.moveCursorToIndex(idx)
	return true
}

// FindPrevious ищет предыдущее вхождение строки
func (e *EditorWidget) FindPrevious(term string) bool {
	if term == "" {
		return false
	}
	content := e.textContent
	startIdx := e.cursorIndex()
	if startIdx > 0 {
		content = content[:startIdx]
	}
	idx := strings.LastIndex(content, term)
	if idx == -1 {
		return false
	}
	e.moveCursorToIndex(idx)
	return true
}

// cursorIndex возвращает индекс курсора в тексте
func (e *EditorWidget) cursorIndex() int {
	lines := strings.Split(e.textContent, "\n")
	idx := 0
	for i := 0; i < e.cursorRow && i < len(lines); i++ {
		idx += len(lines[i]) + 1
	}
	idx += e.cursorCol
	return idx
}

// moveCursorToIndex перемещает курсор к указанному индексу
func (e *EditorWidget) moveCursorToIndex(idx int) {
	if idx < 0 {
		idx = 0
	}
	before := e.textContent[:idx]
	e.cursorRow = strings.Count(before, "\n")
	lastNL := strings.LastIndex(before, "\n")
	if lastNL >= 0 {
		e.cursorCol = idx - lastNL - 1
	} else {
		e.cursorCol = idx
	}
	e.content.CursorRow = e.cursorRow
	e.content.CursorColumn = e.cursorCol
}

// GetWordAtCursor возвращает слово под курсором
func (e *EditorWidget) GetWordAtCursor() string {
	line := e.GetCurrentLine()
	if line == "" {
		return ""
	}
	if e.cursorCol > len(line) {
		return ""
	}
	re := regexp.MustCompile(`[_A-Za-z0-9]+`)
	indices := re.FindAllStringIndex(line, -1)
	for _, loc := range indices {
		if e.cursorCol >= loc[0] && e.cursorCol <= loc[1] {
			e.selectionStart = TextPosition{Row: e.cursorRow, Col: loc[0]}
			e.selectionEnd = TextPosition{Row: e.cursorRow, Col: loc[1]}
			return line[loc[0]:loc[1]]
		}
	}
	return ""
}

// DefinitionLocation представляет местоположение определения
type DefinitionLocation struct {
	Line   int
	Column int
}

// FindDefinition ищет определение слова
func (e *EditorWidget) FindDefinition(word string) *DefinitionLocation {
	if word == "" {
		return nil
	}
	lines := strings.Split(e.textContent, "\n")
	pattern := regexp.MustCompile("\\b" + regexp.QuoteMeta(word) + "\\b")
	for i, line := range lines {
		if loc := pattern.FindStringIndex(line); loc != nil {
			return &DefinitionLocation{Line: i, Column: loc[0]}
		}
	}
	return nil
}

// GoToPosition перемещает курсор в указанную позицию
func (e *EditorWidget) GoToPosition(line, column int) {
	lines := strings.Split(e.textContent, "\n")
	if line < 0 || line >= len(lines) {
		return
	}
	if column < 0 {
		column = 0
	}
	if column > len(lines[line]) {
		column = len(lines[line])
	}
	e.cursorRow = line
	e.cursorCol = column
	e.content.CursorRow = line
	e.content.CursorColumn = column
	e.scrollContainer.ScrollToOffset(fyne.NewPos(0, float32(line)*theme.TextSize()))
}

// FoldCurrentBlock добавляет текущую строку в свернутые диапазоны
func (e *EditorWidget) FoldCurrentBlock() {
	e.toggleFold(e.cursorRow)
}

// UnfoldCurrentBlock раскрывает текущий свернутый блок
func (e *EditorWidget) UnfoldCurrentBlock() {
	if _, ok := e.foldedRanges[e.cursorRow]; ok {
		e.toggleFold(e.cursorRow)
	}
}

// FoldAll сворачивает все возможные блоки
func (e *EditorWidget) FoldAll() {
	lines := strings.Split(e.textContent, "\n")
	for i := 0; i < len(lines); i++ {
		e.toggleFold(i)
	}
}

// UnfoldAll раскрывает весь текст
func (e *EditorWidget) UnfoldAll() {
	rows := make([]int, 0, len(e.foldedRanges))
	for r := range e.foldedRanges {
		rows = append(rows, r)
	}
	sort.Ints(rows)
	for _, r := range rows {
		e.toggleFold(r)
	}
}

// GetFullText возвращает текст с раскрытыми свернутыми блоками
func (e *EditorWidget) GetFullText() string {
	if len(e.foldedRanges) == 0 {
		return e.textContent
	}
	lines := strings.Split(e.textContent, "\n")
	rows := make([]int, 0, len(e.foldedRanges))
	for r := range e.foldedRanges {
		rows = append(rows, r)
	}
	sort.Ints(rows)
	for i := len(rows) - 1; i >= 0; i-- {
		fr := e.foldedRanges[rows[i]]
		start := fr.Start
		if start+1 < len(lines) && strings.TrimSpace(lines[start+1]) == "/*...*/" {
			lines = append(lines[:start+1], lines[start+2:]...)
		}
		if start+1 <= len(lines) {
			lines = append(lines[:start+1], append(fr.Lines, lines[start+1:]...)...)
		}
	}
	return strings.Join(lines, "\n")
}

// SetVimMode устанавливает текущий Vim режим
func (e *EditorWidget) SetVimMode(mode VimMode) {
	e.vimMode = mode
}

// applySyntaxHighlighting применяет подсветку синтаксиса
func (e *EditorWidget) applySyntaxHighlighting() {
	// Всегда синхронизируем Entry с текущим текстом
	fyne.Do(func() {
		e.content.SetText(e.textContent)
	})

	if e.lexer == nil {
		return
	}

	// Проверяем кэш
	cacheKey := fmt.Sprintf("%s_%d", e.filePath, len(e.textContent))
	if tokens, exists := e.syntaxCache[cacheKey]; exists {
		e.syntaxTokens = tokens
	} else {
		// Токенизируем код
		iterator, err := e.lexer.Tokenise(nil, e.textContent)
		if err != nil {
			return
		}

		e.syntaxTokens = iterator.Tokens()
		e.syntaxCache[cacheKey] = e.syntaxTokens

		// Ограничиваем размер кэша
		if len(e.syntaxCache) > 100 {
			// Очищаем старые записи
			for k := range e.syntaxCache {
				delete(e.syntaxCache, k)
				break
			}
		}
	}

	// Преобразуем токены в RichText
	e.applyTokensToRichText()
}

// applyTokensToRichText применяет токены к RichText виджету
func (e *EditorWidget) applyTokensToRichText() {
	segments := []widget.RichTextSegment{}
	index := 0

	for _, token := range e.syntaxTokens {
		if token.Value == "" {
			continue
		}

		style := widget.RichTextStyle{Inline: true, ColorName: e.getTokenColor(token.Type)}
		tokenStart := index
		tokenEnd := index + len(token.Value)
		if e.config != nil && e.config.Editor.HighlightCurrentWord && e.isIndexInSearchResults(tokenStart, tokenEnd) {
			style.ColorName = theme.ColorNameWarning
			style.TextStyle = fyne.TextStyle{Bold: true}
		}

		segment := &widget.TextSegment{
			Text:  token.Value,
			Style: style,
		}
		segments = append(segments, segment)
		index += len(token.Value)
	}

	// Обновление содержимого RichText должно выполняться в главном потоке UI
	fyne.Do(func() {
		e.richContent.Segments = segments
		e.richContent.Refresh()
	})
}

// getTokenColor возвращает цвет для типа токена
func (e *EditorWidget) getTokenColor(tokenType chroma.TokenType) fyne.ThemeColorName {
	switch {
	case tokenType.InCategory(chroma.Keyword):
		return theme.ColorNamePrimary
	case tokenType.InCategory(chroma.String):
		return theme.ColorNameSuccess
	case tokenType.InCategory(chroma.Comment):
		return theme.ColorNameDisabled
	case tokenType.InCategory(chroma.LiteralNumber), tokenType.InCategory(chroma.Number):
		return theme.ColorNameWarning
	case tokenType.InCategory(chroma.Name):
		return theme.ColorNameForeground
	default:
		return theme.ColorNameForeground
	}
}

// updateBracketMatching обновляет подсветку парных скобок
func (e *EditorWidget) updateBracketMatching() {
	e.matchingBrackets = make(map[int]int)
	e.bracketPairs = []BracketPair{}

	stack := []TextPosition{}

	lines := strings.Split(e.textContent, "\n")

	for row, line := range lines {
		runes := []rune(line)
		for col, char := range runes {
			pos := TextPosition{Row: row, Col: col}

			switch char {
			case '(', '[', '{':
				stack = append(stack, pos)
			case ')', ']', '}':
				if len(stack) > 0 {
					openPos := stack[len(stack)-1]
					stack = stack[:len(stack)-1]

					openLine := []rune(lines[openPos.Row])
					if openPos.Col < len(openLine) && e.bracketTypesMatch(openLine[openPos.Col], char) {
						pair := BracketPair{
							Open:      openPos,
							Close:     pos,
							Type:      char,
							IsMatched: true,
						}
						e.bracketPairs = append(e.bracketPairs, pair)

						// Добавляем в карту для быстрого поиска
						openIndex := e.positionToIndex(openPos)
						closeIndex := e.positionToIndex(pos)
						e.matchingBrackets[openIndex] = closeIndex
						e.matchingBrackets[closeIndex] = openIndex
					}
				}
			}
		}
	}
}

// bracketTypesMatch проверяет соответствие типов скобок
func (e *EditorWidget) bracketTypesMatch(open, close rune) bool {
	pairs := map[rune]rune{
		'(': ')',
		'[': ']',
		'{': '}',
	}
	return pairs[open] == close
}

// positionToIndex преобразует позицию в индекс
func (e *EditorWidget) positionToIndex(pos TextPosition) int {
	lines := strings.Split(e.textContent, "\n")
	index := 0

	for i := 0; i < pos.Row && i < len(lines); i++ {
		index += len(lines[i]) + 1 // +1 for newline
	}

	if pos.Row < len(lines) && pos.Col < len(lines[pos.Row]) {
		index += pos.Col
	}

	return index
}

// updateCodeFolding обновляет фолдинг кода
func (e *EditorWidget) updateCodeFolding() {
	lines := strings.Split(e.textContent, "\n")
	e.indentGuides = []IndentGuide{}

	// Анализируем отступы и создаем направляющие
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		indent := e.getIndentLevel(line)
		if indent > 0 {
			guide := IndentGuide{
				Row:        i,
				Col:        indent * 4, // Предполагаем 4 пробела на уровень
				Level:      indent,
				IsVertical: true,
				Label:      e.getBlockLabel(lines, i),
				OnClick: func() {
					e.toggleFold(i)
				},
			}
			e.indentGuides = append(e.indentGuides, guide)
		}
	}
}

// getIndentLevel возвращает уровень отступа строки
func (e *EditorWidget) getIndentLevel(line string) int {
	level := 0
	for _, char := range line {
		if char == ' ' {
			level++
		} else if char == '\t' {
			level += 4 // Табуляция = 4 пробела
		} else {
			break
		}
	}
	return level / 4 // Делим на 4 для получения уровня
}

// getBlockLabel возвращает метку блока кода
func (e *EditorWidget) getBlockLabel(lines []string, startRow int) string {
	line := strings.TrimSpace(lines[startRow])

	// Определяем тип блока
	if strings.Contains(line, "func ") {
		return "function"
	} else if strings.Contains(line, "for ") {
		return "for loop"
	} else if strings.Contains(line, "while ") {
		return "while loop"
	} else if strings.Contains(line, "if ") {
		return "if block"
	} else if strings.Contains(line, "switch ") {
		return "switch"
	} else if strings.Contains(line, "class ") {
		return "class"
	}

	return "block"
}

// toggleFold переключает свертывание блока кода
func (e *EditorWidget) toggleFold(row int) {
	lines := strings.Split(e.textContent, "\n")
	if fold, exists := e.foldedRanges[row]; exists && fold.IsFolded {
		before := lines[:row+1]
		after := lines[row+1:]
		if len(after) > 0 && strings.TrimSpace(after[0]) == "/*...*/" {
			after = after[1:]
		}
		expanded := append(before, append(fold.Lines, after...)...)
		e.textContent = strings.Join(expanded, "\n")
		delete(e.foldedRanges, row)
	} else {
		endRow := e.findBlockEnd(row)
		if endRow > row {
			hidden := append([]string(nil), lines[row+1:endRow+1]...)
			e.foldedRanges[row] = FoldRange{
				Start:       row,
				End:         endRow,
				IsFolded:    true,
				IndentLevel: e.getIndentLevel(lines[row]),
				Label:       e.getBlockLabel(lines, row),
				Lines:       hidden,
			}
			collapsed := append(lines[:row+1], append([]string{"/*...*/"}, lines[endRow+1:]...)...)
			e.textContent = strings.Join(collapsed, "\n")
		}
	}

	fyne.Do(func() {
		e.content.SetText(e.textContent)
	})
	e.updateLineNumbers()
	e.applySyntaxHighlighting()
	e.updateIndentGuides()
}

// findBlockEnd находит конец блока кода
func (e *EditorWidget) findBlockEnd(startRow int) int {
	lines := strings.Split(e.textContent, "\n")
	if startRow >= len(lines) {
		return startRow
	}

	startIndent := e.getIndentLevel(lines[startRow])

	for i := startRow + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			continue // Пропускаем пустые строки
		}

		currentIndent := e.getIndentLevel(line)
		if currentIndent <= startIndent {
			return i - 1
		}
	}

	return len(lines) - 1
}

// updateIndentGuides обновляет направляющие отступов
func (e *EditorWidget) updateIndentGuides() {
	if e.indentContainer == nil || e.config == nil || !e.config.Editor.IndentGuides {
		return
	}

	charWidth := fyne.MeasureString(" ", theme.TextSize()).Width
	lineHeight := fyne.MeasureString("M", theme.TextSize()).Height

	fyne.Do(func() {
		e.indentContainer.Objects = nil
		for _, guide := range e.indentGuides {
			x := float32(guide.Level) * 4 * charWidth
			y := float32(guide.Row) * lineHeight
			line := canvas.NewLine(e.colors.LineNumber)
			line.StrokeWidth = 1
			line.Position1 = fyne.NewPos(x, y)
			line.Position2 = fyne.NewPos(x, y+lineHeight)
			e.indentContainer.Add(line)
		}
		e.indentContainer.Refresh()
	})
}

// parseClickableElements парсит интерактивные элементы в коде
func (e *EditorWidget) parseClickableElements() {
	e.clickableRanges = []ClickableRange{}

	// Парсим импорты
	e.parseImports()

	// Парсим URL
	e.parseURLs()

	// Парсим файловые пути
	e.parseFilePaths()

	// Парсим команды
	e.parseCommands()

	// Парсим цветовые коды
	e.parseColors()

	// Парсим функции и переменные
	e.parseFunctionsAndVariables()

	// Парсим TODO комментарии
	e.parseTODOs()
}

// parseImports парсит импорты в коде
func (e *EditorWidget) parseImports() {
	lines := strings.Split(e.textContent, "\n")

	for row, line := range lines {
		// Go imports
		if match := regexp.MustCompile(`import\s+"([^"]+)"`).FindStringSubmatch(line); len(match) > 1 {
			importPath := match[1]
			start := strings.Index(line, `"`) + 1
			end := start + len(importPath)

			clickable := ClickableRange{
				Range: TextRange{
					Start: TextPosition{Row: row, Col: start},
					End:   TextPosition{Row: row, Col: end},
					Text:  importPath,
				},
				Type:    ClickableImport,
				Target:  e.getImportURL(importPath, "go"),
				Tooltip: fmt.Sprintf("Go to %s documentation", importPath),
				OnClick: func() {
					e.openURL(e.getImportURL(importPath, "go"))
				},
			}
			e.clickableRanges = append(e.clickableRanges, clickable)
		}

		// Python imports
		if match := regexp.MustCompile(`(?:from\s+(\S+)\s+)?import\s+(\S+)`).FindStringSubmatch(line); len(match) > 1 {
			var importName string
			if match[1] != "" {
				importName = match[1]
			} else {
				importName = match[2]
			}

			start := strings.Index(line, importName)
			if start >= 0 {
				clickable := ClickableRange{
					Range: TextRange{
						Start: TextPosition{Row: row, Col: start},
						End:   TextPosition{Row: row, Col: start + len(importName)},
						Text:  importName,
					},
					Type:    ClickableImport,
					Target:  e.getImportURL(importName, "python"),
					Tooltip: fmt.Sprintf("Go to %s documentation", importName),
					OnClick: func() {
						e.openURL(e.getImportURL(importName, "python"))
					},
				}
				e.clickableRanges = append(e.clickableRanges, clickable)
			}
		}
	}
}

// parseURLs парсит URL в коде
func (e *EditorWidget) parseURLs() {
	urlRegex := regexp.MustCompile(`https?://[^\s\)"]+`)
	lines := strings.Split(e.textContent, "\n")

	for row, line := range lines {
		matches := urlRegex.FindAllStringIndex(line, -1)
		for _, match := range matches {
			url := line[match[0]:match[1]]

			clickable := ClickableRange{
				Range: TextRange{
					Start: TextPosition{Row: row, Col: match[0]},
					End:   TextPosition{Row: row, Col: match[1]},
					Text:  url,
				},
				Type:    ClickableURL,
				Target:  url,
				Tooltip: fmt.Sprintf("Open %s", url),
				OnClick: func() {
					e.openURL(url)
				},
			}
			e.clickableRanges = append(e.clickableRanges, clickable)
		}
	}
}

// parseFilePaths парсит файловые пути
func (e *EditorWidget) parseFilePaths() {
	// Простой паттерн для файловых путей
	pathRegex := regexp.MustCompile(`[A-Za-z]:\\[\w\\\.-]+\.\w+|\.{0,2}/[\w/.-]+\.\w+`)
	lines := strings.Split(e.textContent, "\n")

	for row, line := range lines {
		matches := pathRegex.FindAllStringIndex(line, -1)
		for _, match := range matches {
			path := line[match[0]:match[1]]

			clickable := ClickableRange{
				Range: TextRange{
					Start: TextPosition{Row: row, Col: match[0]},
					End:   TextPosition{Row: row, Col: match[1]},
					Text:  path,
				},
				Type:    ClickableFile,
				Target:  path,
				Tooltip: fmt.Sprintf("Open file %s", path),
				OnClick: func() {
					e.openFile(path)
				},
			}
			e.clickableRanges = append(e.clickableRanges, clickable)
		}
	}
}

// parseCommands парсит команды PowerShell/CMD
func (e *EditorWidget) parseCommands() {
	commandRegex := regexp.MustCompile(`(?:ps>|cmd>|\$)\s*([^\n\r]+)`)
	lines := strings.Split(e.textContent, "\n")

	for row, line := range lines {
		matches := commandRegex.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) > 1 {
				command := strings.TrimSpace(match[1])
				start := strings.Index(line, command)

				if start >= 0 {
					clickable := ClickableRange{
						Range: TextRange{
							Start: TextPosition{Row: row, Col: start},
							End:   TextPosition{Row: row, Col: start + len(command)},
							Text:  command,
						},
						Type:    ClickableCommand,
						Target:  command,
						Tooltip: fmt.Sprintf("Run command: %s", command),
						OnClick: func() {
							e.runCommand(command)
						},
					}
					e.clickableRanges = append(e.clickableRanges, clickable)
				}
			}
		}
	}
}

// parseColors парсит цветовые коды
func (e *EditorWidget) parseColors() {
	colorRegex := regexp.MustCompile(`#[0-9A-Fa-f]{6}|#[0-9A-Fa-f]{3}|rgb\(\s*\d+\s*,\s*\d+\s*,\s*\d+\s*\)`)
	lines := strings.Split(e.textContent, "\n")

	for row, line := range lines {
		matches := colorRegex.FindAllStringIndex(line, -1)
		for _, match := range matches {
			colorCode := line[match[0]:match[1]]

			clickable := ClickableRange{
				Range: TextRange{
					Start: TextPosition{Row: row, Col: match[0]},
					End:   TextPosition{Row: row, Col: match[1]},
					Text:  colorCode,
				},
				Type:    ClickableColor,
				Target:  colorCode,
				Tooltip: fmt.Sprintf("Color: %s", colorCode),
				OnClick: func() {
					e.showColorPreview(colorCode)
				},
			}
			e.clickableRanges = append(e.clickableRanges, clickable)
		}
	}
}

// parseFunctionsAndVariables парсит функции и переменные
func (e *EditorWidget) parseFunctionsAndVariables() {
	// Простые паттерны для функций (можно расширить)
	funcRegex := regexp.MustCompile(`\b([a-zA-Z_][a-zA-Z0-9_]*)\s*\(`)
	lines := strings.Split(e.textContent, "\n")

	for row, line := range lines {
		matches := funcRegex.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) > 1 {
				funcName := match[1]
				start := strings.Index(line, funcName)

				if start >= 0 {
					clickable := ClickableRange{
						Range: TextRange{
							Start: TextPosition{Row: row, Col: start},
							End:   TextPosition{Row: row, Col: start + len(funcName)},
							Text:  funcName,
						},
						Type:    ClickableFunction,
						Target:  funcName,
						Tooltip: fmt.Sprintf("Go to function %s", funcName),
						OnClick: func() {
							e.goToDefinition(funcName)
						},
					}
					e.clickableRanges = append(e.clickableRanges, clickable)
				}
			}
		}
	}
}

// parseTODOs парсит TODO комментарии в коде
func (e *EditorWidget) parseTODOs() {
	analyzer := NewCodeAnalyzer()
	todos := analyzer.FindTODOs(e.textContent)

	for _, todo := range todos {
		row := todo.Line - 1
		startCol := todo.Column - 1
		endCol := startCol + len(todo.Text)

		clickable := ClickableRange{
			Range: TextRange{
				Start: TextPosition{Row: row, Col: startCol},
				End:   TextPosition{Row: row, Col: endCol},
				Text:  todo.Text,
			},
			Type:    ClickableTODO,
			Target:  todo.Message,
			Tooltip: fmt.Sprintf("%s: %s", todo.Type, todo.Message),
			OnClick: func() {
				e.showTODOList()
			},
		}
		e.clickableRanges = append(e.clickableRanges, clickable)
	}
}

// showTODOList отображает список TODO комментариев и позволяет перейти к выбранному
func (e *EditorWidget) showTODOList() {
	analyzer := NewCodeAnalyzer()
	todos := analyzer.FindTODOs(e.textContent)

	windows := fyne.CurrentApp().Driver().AllWindows()
	if len(windows) == 0 {
		return
	}
	win := windows[0]

	if len(todos) == 0 {
		dialog.ShowInformation("TODOs", "No TODOs found", win)
		return
	}

	todoNames := make([]string, len(todos))
	for i, t := range todos {
		todoNames[i] = fmt.Sprintf("%s: %s (line %d)", t.Type, t.Message, t.Line)
	}

	todoList := widget.NewList(
		func() int { return len(todoNames) },
		func() fyne.CanvasObject { return widget.NewLabel("TODO") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			o.(*widget.Label).SetText(todoNames[i])
		},
	)

	todoList.OnSelected = func(id widget.ListItemID) {
		if id < len(todos) {
			cmd := &GoToLineCommand{lineNumber: todos[id].Line}
			_ = cmd.Execute(e)
		}
	}

	todoDialog := dialog.NewCustom("TODOs", "Close", container.NewScroll(todoList), win)
	todoDialog.Resize(fyne.NewSize(400, 300))
	todoDialog.Show()
}

// getImportURL возвращает URL документации для импорта
func (e *EditorWidget) getImportURL(importPath, language string) string {
	switch language {
	case "go":
		if strings.Contains(importPath, "github.com") || strings.Contains(importPath, "golang.org") {
			return fmt.Sprintf("https://pkg.go.dev/%s", importPath)
		}
		return fmt.Sprintf("https://golang.org/pkg/%s/", importPath)
	case "python":
		return fmt.Sprintf("https://docs.python.org/3/library/%s.html", importPath)
	case "rust":
		return fmt.Sprintf("https://doc.rust-lang.org/std/%s/", importPath)
	case "java":
		return fmt.Sprintf("https://docs.oracle.com/javase/8/docs/api/%s.html",
			strings.ReplaceAll(importPath, ".", "/"))
	default:
		return ""
	}
}

// Utility functions

// onTextChanged вызывается при изменении текста
func (e *EditorWidget) onTextChanged() {
	e.isDirty = true
	e.resetAutoSaveTimer()

	// Обновляем подсветку синтаксиса (инкрементально)
	go func() {
		time.Sleep(250 * time.Millisecond) // Debounce
		e.updateDisplay()
	}()

	if e.onContentChanged != nil {
		e.onContentChanged(e.textContent)
	}
}

// startFileWatcher запускает наблюдение за файлом
func (e *EditorWidget) startFileWatcher() {
	if e.filePath == "" {
		return
	}

	e.stopFileWatcher() // Останавливаем предыдущий watcher

	var err error
	e.fileWatcher, err = fsnotify.NewWatcher()
	if err != nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	e.watcherCancel = cancel

	go func() {
		defer e.fileWatcher.Close()

		for {
			select {
			case event, ok := <-e.fileWatcher.Events:
				if !ok {
					return
				}

				if event.Op&fsnotify.Write == fsnotify.Write {
					e.handleExternalFileChange()
				}

			case err, ok := <-e.fileWatcher.Errors:
				if !ok {
					return
				}
				fmt.Printf("File watcher error: %v\n", err)

			case <-ctx.Done():
				return
			}
		}
	}()

	e.fileWatcher.Add(e.filePath)
}

// stopFileWatcher останавливает наблюдение за файлом
func (e *EditorWidget) stopFileWatcher() {
	if e.watcherCancel != nil {
		e.watcherCancel()
	}
	if e.fileWatcher != nil {
		e.fileWatcher.Close()
		e.fileWatcher = nil
	}
}

// handleExternalFileChange обрабатывает внешнее изменение файла
func (e *EditorWidget) handleExternalFileChange() {
	if e.filePath == "" {
		return
	}

	info, err := os.Stat(e.filePath)
	if err != nil {
		return
	}

	// Проверяем что файл действительно изменился
	if !info.ModTime().After(e.lastModified) {
		return
	}

	modTime := info.ModTime()
	// Обновляем время модификации сразу, чтобы избежать повторных диалогов
	e.lastModified = modTime

	// Все UI-операции должны выполняться в главном потоке
	fyne.Do(func() {
		win := fyne.CurrentApp().Driver().AllWindows()[0]
		message := "File has been modified outside the editor."
		if e.isDirty {
			message += "\nReloading will discard your unsaved changes."
		}

		dialog.ShowConfirm("File Changed", message+"\nDo you want to reload it?", func(reload bool) {
			if reload {
				if err := e.LoadFile(e.filePath); err != nil {
					dialog.ShowError(err, win)
				}
			}
		}, win)
	})
}

// CreateObject реализует интерфейс fyne.Widget
func (e *EditorWidget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(e.mainContainer)
}

// MinimapColors возвращает цветовую схему для миникарты.
func (e *EditorWidget) MinimapColors() MinimapColors {
	return MinimapColors{
		Background:     editorBackground,
		Text:           textPrimary,
		Comment:        syntaxComment,
		String:         syntaxString,
		Keyword:        syntaxKeyword,
		Number:         syntaxNumber,
		Function:       syntaxFunction,
		Viewport:       color.NRGBA{accentBlue.R, accentBlue.G, accentBlue.B, 0x40},
		ViewportBorder: accentBlue,
		ScrollBar:      scrollbarThumb,
		LineNumber:     textSecondary,
		Selection:      editorSelection,
	}
}

// Методы для внешнего API
func (e *EditorWidget) GetContent() string  { return e.textContent }
func (e *EditorWidget) GetFilePath() string { return e.filePath }
func (e *EditorWidget) GetFileName() string { return e.fileName }

func (e *EditorWidget) showContextMenu(pos fyne.Position) {
	c := fyne.CurrentApp().Driver().CanvasForObject(e.content)
	if c == nil {
		return
	}
	menu := fyne.NewMenu("",
		fyne.NewMenuItem("Cut", func() {
			e.content.TypedShortcut(&fyne.ShortcutCut{Clipboard: fyne.CurrentApp().Clipboard()})
		}),
		fyne.NewMenuItem("Copy", func() {
			e.content.TypedShortcut(&fyne.ShortcutCopy{Clipboard: fyne.CurrentApp().Clipboard()})
		}),
		fyne.NewMenuItem("Paste", func() {
			e.content.TypedShortcut(&fyne.ShortcutPaste{Clipboard: fyne.CurrentApp().Clipboard()})
		}),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Select All", func() {
			e.SelectAll()
		}),
	)
	widget.NewPopUpMenu(menu, c).ShowAtPosition(pos)
}
func (e *EditorWidget) openURL(rawurl string) {
	u, err := url.Parse(rawurl)
	if err != nil {
		return
	}
	_ = fyne.CurrentApp().OpenURL(u)
}

func (e *EditorWidget) openFile(path string) {
	if path == "" {
		return
	}
	if err := e.LoadFile(path); err == nil {
		if e.onFileChanged != nil {
			e.onFileChanged(path)
		}
	}
}

func (e *EditorWidget) runCommand(command string) {
	if command == "" {
		return
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Start()
}

func (e *EditorWidget) showColorPreview(col string) {
	c, err := parseColor(col)
	if err != nil {
		return
	}
	rect := canvas.NewRectangle(c)
	rect.SetMinSize(fyne.NewSize(100, 100))
	dialog.NewCustom("Color preview", "Close", rect, fyne.CurrentApp().Driver().AllWindows()[0]).Show()
}

func (e *EditorWidget) goToDefinition(name string) {
	lines := strings.Split(e.textContent, "\n")
	for i, line := range lines {
		if idx := strings.Index(line, name); idx >= 0 {
			e.content.CursorRow = i
			e.content.CursorColumn = idx
			e.content.Refresh()
			e.cursorRow = i
			e.cursorCol = idx
			e.scrollContainer.ScrollToOffset(fyne.NewPos(0, float32(i)*theme.TextSize()))
			break
		}
	}
}

// Utility functions

func parseColor(s string) (color.Color, error) {
	if strings.HasPrefix(s, "#") {
		s = strings.TrimPrefix(s, "#")
		var r, g, b uint8
		switch len(s) {
		case 3:
			_, err := fmt.Sscanf(s, "%1x%1x%1x", &r, &g, &b)
			if err != nil {
				return nil, err
			}
			r *= 17
			g *= 17
			b *= 17
		case 6:
			_, err := fmt.Sscanf(s, "%02x%02x%02x", &r, &g, &b)
			if err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("invalid color format")
		}
		return color.NRGBA{R: r, G: g, B: b, A: 255}, nil
	}
	if strings.HasPrefix(strings.ToLower(s), "rgb(") {
		var r, g, b int
		_, err := fmt.Sscanf(s, "rgb(%d,%d,%d)", &r, &g, &b)
		if err != nil {
			return nil, err
		}
		return color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}, nil
	}
	return nil, fmt.Errorf("unsupported color format")
}

func isASCII(data []byte) bool {
	for _, b := range data {
		if b > 127 {
			return false
		}
	}
	return true
}

func detectLineEnding(data []byte) string {
	content := string(data)
	if strings.Contains(content, "\r\n") {
		return "\r\n" // Windows
	} else if strings.Contains(content, "\n") {
		return "\n" // Unix
	} else if strings.Contains(content, "\r") {
		return "\r" // Old Mac
	}
	return "\r\n" // Default Windows
}
