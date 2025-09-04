package main

import (
	"bufio"
	"context"
	"fmt"
	"fyne.io/fyne/v2/theme"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/fsnotify/fsnotify"
)

// EditorWidget - основной виджет редактора с полным функционалом
type EditorWidget struct {
	widget.BaseWidget

	// Основные компоненты
	content         *widget.RichText
	lineNumbers     *widget.List
	scrollContainer *container.Scroll
	mainContainer   *container.Border

	// Состояние файла
	filePath     string
	fileName     string
	isDirty      bool
	isReadOnly   bool
	lastModified time.Time
	encoding     string
	lineEnding   string

	// Содержимое и позиция
	textContent    string
	cursorRow      int
	cursorCol      int
	selectionStart TextPosition
	selectionEnd   TextPosition

	// Подсветка синтаксиса
	lexer        chroma.Lexer
	formatter    chroma.Formatter
	style        *chroma.Style
	syntaxTokens []chroma.Token

	// Интерактивные элементы
	clickableRanges []ClickableRange
	hoveredRange    *ClickableRange

	// Фолдинг кода
	foldedRanges map[int]FoldRange
	indentGuides []IndentGuide

	// Поиск и выделение
	searchTerm    string
	searchResults []TextRange
	currentSearch int
	selectedVar   string
	varHighlights []TextRange

	// Bracket matching
	matchingBrackets map[int]int
	bracketPairs     []BracketPair

	// Настройки
	config *Config
	colors EditorColors

	// Автосохранение и file watching
	autoSaveTimer *time.Timer
	fileWatcher   *fsnotify.Watcher
	watcherCancel context.CancelFunc

	// Производительность
	renderMutex sync.RWMutex
	syntaxCache map[string][]chroma.Token

	// Callbacks
	onContentChanged func(string)
	onCursorChanged  func(int, int)
	onFileChanged    func(string)
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
func NewEditor(config *Config) *EditorWidget {
	editor := &EditorWidget{
		config:           config,
		colors:           GetEditorColors(config.Theme == "dark"),
		foldedRanges:     make(map[int]FoldRange),
		matchingBrackets: make(map[int]int),
		syntaxCache:      make(map[string][]chroma.Token),
		encoding:         "UTF-8",
		lineEnding:       "\r\n", // Windows default
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
	// Создаем основной текстовый виджет
	e.content = widget.NewRichText()
	e.content.Wrapping = fyne.TextWrap(e.config.WordWrap)

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
	editorContent := container.NewHSplit(e.lineNumbers, e.content)
	editorContent.SetOffset(0.05) // 5% для номеров строк

	e.scrollContainer = container.NewScroll(editorContent)
	e.scrollContainer.SetMinSize(fyne.NewSize(800, 600))

	// Основной контейнер
	e.mainContainer = container.NewBorder(nil, nil, nil, nil, e.scrollContainer)
}

// setupSyntaxHighlighter настраивает подсветку синтаксиса
func (e *EditorWidget) setupSyntaxHighlighter() {
	// Получаем стиль для темной/светлой темы
	if e.config.Theme == "dark" {
		e.style = styles.Get("monokai")
	} else {
		e.style = styles.Get("github")
	}

	// Создаем formatter для вывода
	e.formatter = formatters.Get("terminal256")
	if e.formatter == nil {
		e.formatter = formatters.Fallback
	}
}

// setupAutoSave настраивает автосохранение
func (e *EditorWidget) setupAutoSave() {
	if !e.config.AutoSave {
		return
	}

	duration := time.Duration(e.config.AutoSaveMinutes) * time.Minute
	e.autoSaveTimer = time.AfterFunc(duration, func() {
		if e.isDirty && e.filePath != "" {
			e.SaveFile()
			e.resetAutoSaveTimer()
		}
	})
}

// resetAutoSaveTimer сбрасывает таймер автосохранения
func (e *EditorWidget) resetAutoSaveTimer() {
	if e.autoSaveTimer != nil {
		e.autoSaveTimer.Stop()
	}
	if e.config.AutoSave {
		duration := time.Duration(e.config.AutoSaveMinutes) * time.Minute
		e.autoSaveTimer = time.AfterFunc(duration, func() {
			if e.isDirty && e.filePath != "" {
				e.SaveFile()
				e.resetAutoSaveTimer()
			}
		})
	}
}

// bindEvents привязывает события
func (e *EditorWidget) bindEvents() {
	// Обработка изменений текста
	e.content.OnChanged = func() {
		e.onTextChanged()
	}

	// Обработка кликов мыши
	e.content.OnTappedSecondary = func(pe *fyne.PointEvent) {
		e.showContextMenu(pe.Position)
	}
}

// LoadFile загружает файл в редактор
func (e *EditorWidget) LoadFile(filepath string) error {
	// Проверяем размер файла
	info, err := os.Stat(filepath)
	if err != nil {
		return fmt.Errorf("cannot access file: %v", err)
	}

	if info.Size() > 100*1024*1024 { // 100MB limit
		return fmt.Errorf("file too large (max 100MB)")
	}

	// Читаем содержимое файла
	content, err := ioutil.ReadFile(filepath)
	if err != nil {
		return fmt.Errorf("cannot read file: %v", err)
	}

	// Определяем кодировку (пока только UTF-8 и ASCII)
	e.encoding = "UTF-8"
	if isASCII(content) {
		e.encoding = "ASCII"
	}

	// Определяем тип переноса строк
	e.lineEnding = detectLineEnding(content)

	// Устанавливаем содержимое
	e.textContent = string(content)
	e.filePath = filepath
	e.fileName = filepath[strings.LastIndex(filepath, "\\")+1:]
	e.lastModified = info.ModTime()
	e.isDirty = false

	// Определяем язык по расширению файла
	e.detectLanguage(filepath)

	// Обновляем отображение
	e.updateDisplay()

	// Запускаем file watcher
	e.startFileWatcher()

	// Парсим интерактивные элементы
	e.parseClickableElements()

	// Callback
	if e.onFileChanged != nil {
		e.onFileChanged(filepath)
	}

	return nil
}

// SaveFile сохраняет файл
func (e *EditorWidget) SaveFile() error {
	if e.filePath == "" {
		return fmt.Errorf("no file path specified")
	}

	// Конвертируем перенос строк
	content := strings.ReplaceAll(e.textContent, "\n", e.lineEnding)

	// Сохраняем файл
	err := ioutil.WriteFile(e.filePath, []byte(content), 0644)
	if err != nil {
		return fmt.Errorf("cannot save file: %v", err)
	}

	// Обновляем состояние
	info, _ := os.Stat(e.filePath)
	e.lastModified = info.ModTime()
	e.isDirty = false

	// Сбрасываем таймер автосохранения
	e.resetAutoSaveTimer()

	return nil
}

// SaveAsFile сохраняет файл с новым именем
func (e *EditorWidget) SaveAsFile(filepath string) error {
	e.filePath = filepath
	e.fileName = filepath[strings.LastIndex(filepath, "\\")+1:]
	return e.SaveFile()
}

// detectLanguage определяет язык программирования по расширению
func (e *EditorWidget) detectLanguage(filepath string) {
	ext := strings.ToLower(filepath[strings.LastIndex(filepath, ".")+1:])

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

// getLineCount возвращает количество строк
func (e *EditorWidget) getLineCount() int {
	if e.textContent == "" {
		return 1
	}
	return len(strings.Split(e.textContent, "\n"))
}

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
func (e *EditorWidget) GetContent() string { return e.textContent }
func (e *EditorWidget) SetContent(text string) {
	e.textContent = text
	e.updateDisplay()
}
func (e *EditorWidget) IsDirty() bool       { return e.isDirty }
func (e *EditorWidget) GetFilePath() string { return e.filePath }
func (e *EditorWidget) GetFileName() string { return e.fileName }

// Заглушки для методов, которые будут реализованы позже
func (e *EditorWidget) showContextMenu(pos fyne.Position) {}
func (e *EditorWidget) openURL(url string)                {}
func (e *EditorWidget) openFile(path string)              {}
func (e *EditorWidget) runCommand(command string)         {}
func (e *EditorWidget) showColorPreview(color string)     {}
func (e *EditorWidget) goToDefinition(name string)        {}

// Utility functions
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
