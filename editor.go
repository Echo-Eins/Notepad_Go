package main

import (
	"context"
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
	"strings"
	"sync"
	"time"
)

// EditorWidget - основной виджет редактора с полным функционалом
type EditorWidget struct {
	widget.BaseWidget

	// Основные компоненты
	content         *widget.Entry    // Изменено с RichText на Entry для редактирования
	richContent     *widget.RichText // Для отображения с подсветкой
	lineNumbers     *widget.List
	scrollContainer *container.Scroll
	mainContainer   *fyne.Container // Изменено с container.Border на *fyne.Container

	// Конфигурация
	config *Config
	colors map[string]color.Color

	// Состояние редактора
	filePath    string
	textContent string
	isDirty     bool
	isReadOnly  bool
	encoding    string
	lineEnding  string

	// Позиция курсора
	cursorRow      int
	cursorCol      int
	selectionStart TextPosition
	selectionEnd   TextPosition

	// История изменений
	undoStack     []Command
	redoStack     []Command
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
	fileWatcher   *fsnotify.Watcher
	watcherCancel context.CancelFunc
	searchResults []SearchResult

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
	indentGuides []IndentGuide
	fileName     string
}

// IsDirty возвращает true, если есть несохраненные изменения
func (e *EditorWidget) IsDirty() bool {
	return e.isDirty
}

// LoadFile загружает файл в редактор
func (e *EditorWidget) LoadFile(path string) error {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	e.filePath = path
	e.fileName = filepath.Base(path)
	e.textContent = string(content)
	e.isDirty = false
	if info, err := os.Stat(path); err == nil {
		e.lastModified = info.ModTime()
	}
	e.detectLanguage()
	e.updateDisplay()
	e.startFileWatcher()

	return nil
}

// SaveFile сохраняет содержимое в файл
func (e *EditorWidget) SaveFile() error {
	if e.filePath == "" {
		return fmt.Errorf("no file path specified")
	}

	err := ioutil.WriteFile(e.filePath, []byte(e.textContent), 0644)
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

// getLineCount возвращает количество строк
func (e *EditorWidget) getLineCount() int {
	if e.textContent == "" {
		return 1
	}
	return len(strings.Split(e.textContent, "\n"))
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

type SearchResult struct {
	Position TextPosition
	Length   int
	Text     string
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
)

// FoldRange представляет свернутый блок кода
type FoldRange struct {
	StartRow    int
	EndRow      int
	IsFolded    bool
	IndentLevel int
	Label       string
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

	// Создаем список номеров строк
	e.lineNumbers = widget.NewList(
		func() int { return e.getLineCount() },
		func() fyne.CanvasObject {
			label := widget.NewLabel("1")
			label.Alignment = fyne.TextAlignTrailing
			return label
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			label := obj.(*widget.Label)
			lineNum := id + 1
			label.SetText(fmt.Sprintf("%d", lineNum))

			// Подсветка текущей строки
			if lineNum == e.cursorRow+1 {
				label.Importance = widget.HighImportance
			} else {
				label.Importance = widget.MediumImportance
			}
		},
	)

	// Настраиваем размеры
	e.lineNumbers.Resize(fyne.NewSize(60, 0))

	// Создаем контейнер с прокруткой
	var editorContent fyne.CanvasObject
	if e.config.Editor.ShowLineNumbers {
		split := container.NewHSplit(e.lineNumbers, e.content)
		split.SetOffset(0.05) // 5% для номеров строк
		editorContent = split
	} else {
		editorContent = e.content
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
	if entry, ok := e.content.(*widget.Entry); ok {
		entry.OnCursorChanged = func() {
			// Обновляем позицию курсора
			e.updateCursorPosition()
		}
	}

	// Для контекстного меню используем TappedSecondary на всем виджете
	e.content.OnTapped = func(event *fyne.PointEvent) {
		// Обработка клика
	}

	// Добавляем обработку правого клика через расширение
	e.ExtendBaseWidget(e)
}

// TappedSecondary обрабатывает правый клик мыши
func (e *EditorWidget) TappedSecondary(event *fyne.PointEvent) {
	e.showContextMenu(event.Position)
}

// updateCursorPosition обновляет внутреннее состояние позиции курсора
func (e *EditorWidget) updateCursorPosition() {
	if entry, ok := e.content.(*widget.Entry); ok {
		e.cursorRow = entry.CursorRow
		e.cursorCol = entry.CursorColumn
		if e.onCursorChanged != nil {
			e.onCursorChanged(e.cursorRow, e.cursorCol)
		}
	}
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

	// Обновляем номера строк
	e.lineNumbers.Refresh()

	// Обновляем содержимое
	e.content.Refresh()
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
	e.content.SelectAll()
}

// GetCursorPosition возвращает позицию курсора
func (e *EditorWidget) GetCursorPosition() TextPosition {
	return TextPosition{
		Row: e.cursorRow,
		Col: e.cursorCol,
	}
}

// applySyntaxHighlighting применяет подсветку синтаксиса
func (e *EditorWidget) applySyntaxHighlighting() {
	if e.lexer == nil {
		e.content.SetText(e.textContent)
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
			e.content.SetText(e.textContent)
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

	for _, token := range e.syntaxTokens {
		if token.Value == "" {
			continue
		}

		style := widget.RichTextStyle{}
		color := e.getTokenColor(token.Type)

		segment := &widget.TextSegment{
			Text:  token.Value,
			Style: style,
		}

		// Устанавливаем цвет (это зависит от реализации Fyne)
		_ = color // TODO: применить цвет к сегменту

		segments = append(segments, segment)
	}

	e.content.Segments = segments
}

// getTokenColor возвращает цвет для типа токена
func (e *EditorWidget) getTokenColor(tokenType chroma.TokenType) fyne.ThemeColorName {
	switch {
	case tokenType.In(chroma.Keyword):
		return theme.ColorNamePrimary
	case tokenType.In(chroma.String):
		return theme.ColorNameSuccess
	case tokenType.In(chroma.Comment):
		return theme.ColorNameDisabled
	case tokenType.In(chroma.Number):
		return theme.ColorNameWarning
	case tokenType.In(chroma.Name):
		return theme.ColorNameForeground
	default:
		return theme.ColorNameForeground
	}
}

// updateBracketMatching обновляет подсветку парных скобок
func (e *EditorWidget) updateBracketMatching() {
	e.matchingBrackets = make(map[int]int)
	e.bracketPairs = []BracketPair{}

	brackets := []rune{'(', ')', '[', ']', '{', '}'}
	stack := []TextPosition{}

	lines := strings.Split(e.textContent, "\n")

	for row, line := range lines {
		for col, char := range line {
			pos := TextPosition{Row: row, Col: col}

			switch char {
			case '(', '[', '{':
				stack = append(stack, pos)
			case ')', ']', '}':
				if len(stack) > 0 {
					openPos := stack[len(stack)-1]
					stack = stack[:len(stack)-1]

					// Проверяем соответствие типов скобок
					if e.bracketTypesMatch(lines[openPos.Row][openPos.Col], char) {
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
	if fold, exists := e.foldedRanges[row]; exists {
		fold.IsFolded = !fold.IsFolded
		e.foldedRanges[row] = fold
	} else {
		// Создаем новый fold
		endRow := e.findBlockEnd(row)
		if endRow > row {
			e.foldedRanges[row] = FoldRange{
				StartRow:    row,
				EndRow:      endRow,
				IsFolded:    true,
				IndentLevel: e.getIndentLevel(strings.Split(e.textContent, "\n")[row]),
				Label:       e.getBlockLabel(strings.Split(e.textContent, "\n"), row),
			}
		}
	}

	e.updateDisplay()
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
	// Реализация зависит от способа отрисовки направляющих в Fyne
	// Пока оставляем заглушку
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
	info, err := os.Stat(e.filePath)
	if err != nil {
		return
	}

	if info.ModTime().After(e.lastModified) {
		// Файл был изменен внешне
		// TODO: показать диалог пользователю о перезагрузке
		// Пока автоматически перезагружаем если нет несохраненных изменений
		if !e.isDirty {
			e.LoadFile(e.filePath)
		}
	}
}

// CreateObject реализует интерфейс fyne.Widget
func (e *EditorWidget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(e.mainContainer)
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
			if entry, ok := e.content.(*widget.Entry); ok {
				entry.CursorRow = i
				entry.CursorColumn = idx
				entry.Refresh()
			}
			e.cursorRow = i
			e.cursorCol = idx
			e.scrollContainer.ScrollTo(fyne.NewPos(0, float32(i)*theme.TextSize()))
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
