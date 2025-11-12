package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/alecthomas/chroma/v2"
	"image/color"
	"strings"
	"sync"
	"time"
)

// EditableRichTextWidget редактируемый RichText widget с подсветкой синтаксиса
// Объединяет функциональность Entry (редактирование) и RichText (подсветка)
// в один компонент для устранения двухслойного рендеринга
type EditableRichTextWidget struct {
	widget.BaseWidget

	// Содержимое
	text  string
	lines []string
	mutex sync.RWMutex

	// Курсор и выделение
	cursorRow      int
	cursorCol      int
	selectionStart TextPosition
	selectionEnd   TextPosition
	selecting      bool

	// Курсор анимация
	cursorVisible bool
	cursorBlink   *time.Ticker
	cursorStop    chan bool

	// Подсветка синтаксиса
	syntaxTokens  []chroma.Token
	syntaxEnabled bool
	lexer         chroma.Lexer
	syntaxStyle   *chroma.Style
	syntaxCache   map[string][]chroma.Token
	syntaxMutex   sync.RWMutex

	// Виртуализация
	firstVisibleLine int
	lastVisibleLine  int
	bufferLines      int
	scrollOffset     float32

	// Метрики текста
	lineHeight float32
	charWidth  float32
	fontSize   float32

	// Цвета
	backgroundColor  color.Color
	textColor        color.Color
	cursorColor      color.Color
	selectionColor   color.Color
	currentLineColor color.Color

	// Рендеринг
	renderCache    map[int]*renderedLine
	cacheEnabled   bool
	maxCacheSize   int
	lastRenderHash uint64

	// Callbacks
	onChanged       func(text string)
	onCursorChanged func(row, col int)
	onKeyPressed    func(event *fyne.KeyEvent) bool

	// Синхронизация скролла
	scrollSync *ScrollSynchronizer
	observerID string

	// Настройки
	tabSize   int
	wrapMode  fyne.TextWrap
	readOnly  bool
	multiLine bool

	// Фокус
	focused bool
}

// renderedLine кэшированная отрендеренная строка
type renderedLine struct {
	lineNumber int
	objects    []fyne.CanvasObject
	hash       uint64
	timestamp  time.Time
}

// NewEditableRichTextWidget создает новый редактируемый RichText виджет
func NewEditableRichTextWidget() *EditableRichTextWidget {
	widget := &EditableRichTextWidget{
		lines:            []string{""},
		cursorRow:        0,
		cursorCol:        0,
		cursorColor:      theme.PrimaryColor(),
		backgroundColor:  theme.BackgroundColor(),
		textColor:        theme.ForegroundColor(),
		selectionColor:   theme.SelectionColor(),
		currentLineColor: color.RGBA{R: 64, G: 64, B: 64, A: 50},
		fontSize:         theme.TextSize(),
		lineHeight:       theme.TextSize() * 1.2,
		charWidth:        MeasureString("M", theme.TextSize()).Width,
		bufferLines:      10,
		renderCache:      make(map[int]*renderedLine),
		syntaxCache:      make(map[string][]chroma.Token),
		cacheEnabled:     true,
		maxCacheSize:     500,
		syntaxEnabled:    true,
		tabSize:          4,
		wrapMode:         fyne.TextWrapOff,
		multiLine:        true,
		observerID:       "editable_richtext_widget",
		cursorStop:       make(chan bool, 1),
	}

	widget.ExtendBaseWidget(widget)
	widget.startCursorBlink()

	return widget
}

// CreateRenderer создает renderer для виджета
func (w *EditableRichTextWidget) CreateRenderer() fyne.WidgetRenderer {
	return &editableRichTextRenderer{
		widget:  w,
		objects: []fyne.CanvasObject{},
	}
}

// SetText устанавливает текст
func (w *EditableRichTextWidget) SetText(text string) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	w.text = text
	w.lines = strings.Split(text, "\n")
	if len(w.lines) == 0 {
		w.lines = []string{""}
	}

	w.clearRenderCache()
	w.applySyntaxHighlighting()

	fyne.Do(func() {
		w.Refresh()
		if w.onChanged != nil {
			w.onChanged(text)
		}
	})
}

// GetText возвращает текущий текст
func (w *EditableRichTextWidget) GetText() string {
	w.mutex.RLock()
	defer w.mutex.RUnlock()
	return w.text
}

// SetSyntaxLexer устанавливает лексер для подсветки синтаксиса
func (w *EditableRichTextWidget) SetSyntaxLexer(lexer chroma.Lexer, style *chroma.Style) {
	w.syntaxMutex.Lock()
	defer w.syntaxMutex.Unlock()

	w.lexer = lexer
	w.syntaxStyle = style
	w.clearRenderCache()
	w.applySyntaxHighlighting()

	fyne.Do(func() {
		w.Refresh()
	})
}

// EnableSyntax включает/выключает подсветку синтаксиса
func (w *EditableRichTextWidget) EnableSyntax(enabled bool) {
	w.syntaxMutex.Lock()
	w.syntaxEnabled = enabled
	w.syntaxMutex.Unlock()

	w.clearRenderCache()
	fyne.Do(func() {
		w.Refresh()
	})
}

// applySyntaxHighlighting применяет подсветку синтаксиса
func (w *EditableRichTextWidget) applySyntaxHighlighting() {
	if !w.syntaxEnabled || w.lexer == nil {
		return
	}

	// Проверяем кэш
	w.syntaxMutex.RLock()
	if cached, ok := w.syntaxCache[w.text]; ok {
		w.syntaxTokens = cached
		w.syntaxMutex.RUnlock()
		return
	}
	w.syntaxMutex.RUnlock()

	// Токенизируем текст
	iterator, err := w.lexer.Tokenise(nil, w.text)
	if err != nil {
		return
	}

	tokens := iterator.Tokens()
	w.syntaxMutex.Lock()
	w.syntaxTokens = tokens

	// Сохраняем в кэш (ограничиваем размер)
	if len(w.syntaxCache) > 100 {
		// Простая стратегия: очищаем кэш полностью
		w.syntaxCache = make(map[string][]chroma.Token)
	}
	w.syntaxCache[w.text] = tokens
	w.syntaxMutex.Unlock()
}

// TypedRune обрабатывает ввод символа
func (w *EditableRichTextWidget) TypedRune(r rune) {
	if w.readOnly || !w.focused {
		return
	}

	w.mutex.Lock()
	defer w.mutex.Unlock()

	// Удаляем выделенный текст если есть
	if w.hasSelection() {
		w.deleteSelection()
	}

	// Вставляем символ
	if w.cursorRow >= len(w.lines) {
		w.cursorRow = len(w.lines) - 1
	}

	line := w.lines[w.cursorRow]
	if w.cursorCol > len(line) {
		w.cursorCol = len(line)
	}

	newLine := line[:w.cursorCol] + string(r) + line[w.cursorCol:]
	w.lines[w.cursorRow] = newLine
	w.cursorCol++

	w.updateTextFromLines()
	w.notifyChanged()
}

// TypedKey обрабатывает нажатие специальных клавиш
func (w *EditableRichTextWidget) TypedKey(key *fyne.KeyEvent) {
	w.handleKey(key.Name, 0) // No modifiers in basic KeyEvent
}

// KeyDown обрабатывает нажатие клавиши с модификаторами (desktop.Keyable)
func (w *EditableRichTextWidget) KeyDown(key *fyne.KeyEvent) {
	w.handleKey(key.Name, 0) // Base implementation
}

// KeyUp обрабатывает отпускание клавиши (desktop.Keyable)
func (w *EditableRichTextWidget) KeyUp(key *fyne.KeyEvent) {
	// Ничего не делаем при отпускании клавиши
}

// handleKey общий обработчик клавиш
func (w *EditableRichTextWidget) handleKey(keyName fyne.KeyName, mods fyne.KeyModifier) {
	if !w.focused {
		return
	}

	w.mutex.Lock()
	defer w.mutex.Unlock()

	switch keyName {
	case fyne.KeyReturn:
		if !w.multiLine || w.readOnly {
			return
		}
		w.handleEnter()

	case fyne.KeyBackspace:
		if w.readOnly {
			return
		}
		w.handleBackspace()

	case fyne.KeyDelete:
		if w.readOnly {
			return
		}
		w.handleDelete()

	case fyne.KeyLeft:
		w.moveCursorLeft(mods)

	case fyne.KeyRight:
		w.moveCursorRight(mods)

	case fyne.KeyUp:
		w.moveCursorUp(mods)

	case fyne.KeyDown:
		w.moveCursorDown(mods)

	case fyne.KeyHome:
		w.moveCursorHome(mods)

	case fyne.KeyEnd:
		w.moveCursorEnd(mods)

	case fyne.KeyPageUp:
		w.moveCursorPageUp(mods)

	case fyne.KeyPageDown:
		w.moveCursorPageDown(mods)

	case fyne.KeyTab:
		if !w.readOnly {
			w.handleTab(mods)
		}
	}

	w.notifyChanged()
}

// handleEnter обрабатывает Enter
func (w *EditableRichTextWidget) handleEnter() {
	if w.hasSelection() {
		w.deleteSelection()
	}

	line := w.lines[w.cursorRow]
	before := line[:w.cursorCol]
	after := line[w.cursorCol:]

	w.lines[w.cursorRow] = before
	w.lines = append(w.lines[:w.cursorRow+1], append([]string{after}, w.lines[w.cursorRow+1:]...)...)

	w.cursorRow++
	w.cursorCol = 0
	w.updateTextFromLines()
}

// handleBackspace обрабатывает Backspace
func (w *EditableRichTextWidget) handleBackspace() {
	if w.hasSelection() {
		w.deleteSelection()
		return
	}

	if w.cursorCol > 0 {
		// Удаляем символ в текущей строке
		line := w.lines[w.cursorRow]
		w.lines[w.cursorRow] = line[:w.cursorCol-1] + line[w.cursorCol:]
		w.cursorCol--
	} else if w.cursorRow > 0 {
		// Объединяем с предыдущей строкой
		prevLine := w.lines[w.cursorRow-1]
		currentLine := w.lines[w.cursorRow]
		w.lines[w.cursorRow-1] = prevLine + currentLine
		w.lines = append(w.lines[:w.cursorRow], w.lines[w.cursorRow+1:]...)
		w.cursorRow--
		w.cursorCol = len(prevLine)
	}

	w.updateTextFromLines()
}

// handleDelete обрабатывает Delete
func (w *EditableRichTextWidget) handleDelete() {
	if w.hasSelection() {
		w.deleteSelection()
		return
	}

	line := w.lines[w.cursorRow]
	if w.cursorCol < len(line) {
		// Удаляем символ в текущей строке
		w.lines[w.cursorRow] = line[:w.cursorCol] + line[w.cursorCol+1:]
	} else if w.cursorRow < len(w.lines)-1 {
		// Объединяем со следующей строкой
		nextLine := w.lines[w.cursorRow+1]
		w.lines[w.cursorRow] = line + nextLine
		w.lines = append(w.lines[:w.cursorRow+1], w.lines[w.cursorRow+2:]...)
	}

	w.updateTextFromLines()
}

// handleTab обрабатывает Tab
func (w *EditableRichTextWidget) handleTab(mods fyne.KeyModifier) {
	if w.hasSelection() {
		// Сдвигаем выделенные строки
		w.indentSelection(mods&fyne.KeyModifierShift == 0)
		return
	}

	// Вставляем табуляцию
	spaces := strings.Repeat(" ", w.tabSize)
	line := w.lines[w.cursorRow]
	w.lines[w.cursorRow] = line[:w.cursorCol] + spaces + line[w.cursorCol:]
	w.cursorCol += w.tabSize
	w.updateTextFromLines()
}

// Движение курсора

func (w *EditableRichTextWidget) moveCursorLeft(mods fyne.KeyModifier) {
	startSel := w.hasSelection()

	if w.cursorCol > 0 {
		w.cursorCol--
	} else if w.cursorRow > 0 {
		w.cursorRow--
		w.cursorCol = len(w.lines[w.cursorRow])
	}

	w.handleSelection(mods, startSel)
	w.notifyCursorChanged()
}

func (w *EditableRichTextWidget) moveCursorRight(mods fyne.KeyModifier) {
	startSel := w.hasSelection()

	line := w.lines[w.cursorRow]
	if w.cursorCol < len(line) {
		w.cursorCol++
	} else if w.cursorRow < len(w.lines)-1 {
		w.cursorRow++
		w.cursorCol = 0
	}

	w.handleSelection(mods, startSel)
	w.notifyCursorChanged()
}

func (w *EditableRichTextWidget) moveCursorUp(mods fyne.KeyModifier) {
	startSel := w.hasSelection()

	if w.cursorRow > 0 {
		w.cursorRow--
		line := w.lines[w.cursorRow]
		if w.cursorCol > len(line) {
			w.cursorCol = len(line)
		}
	}

	w.handleSelection(mods, startSel)
	w.notifyCursorChanged()
}

func (w *EditableRichTextWidget) moveCursorDown(mods fyne.KeyModifier) {
	startSel := w.hasSelection()

	if w.cursorRow < len(w.lines)-1 {
		w.cursorRow++
		line := w.lines[w.cursorRow]
		if w.cursorCol > len(line) {
			w.cursorCol = len(line)
		}
	}

	w.handleSelection(mods, startSel)
	w.notifyCursorChanged()
}

func (w *EditableRichTextWidget) moveCursorHome(mods fyne.KeyModifier) {
	startSel := w.hasSelection()

	if mods&fyne.KeyModifierControl != 0 {
		w.cursorRow = 0
		w.cursorCol = 0
	} else {
		w.cursorCol = 0
	}

	w.handleSelection(mods, startSel)
	w.notifyCursorChanged()
}

func (w *EditableRichTextWidget) moveCursorEnd(mods fyne.KeyModifier) {
	startSel := w.hasSelection()

	if mods&fyne.KeyModifierControl != 0 {
		w.cursorRow = len(w.lines) - 1
		w.cursorCol = len(w.lines[w.cursorRow])
	} else {
		w.cursorCol = len(w.lines[w.cursorRow])
	}

	w.handleSelection(mods, startSel)
	w.notifyCursorChanged()
}

func (w *EditableRichTextWidget) moveCursorPageUp(mods fyne.KeyModifier) {
	startSel := w.hasSelection()

	linesPerPage := 20 // TODO: вычислить из viewport size
	w.cursorRow -= linesPerPage
	if w.cursorRow < 0 {
		w.cursorRow = 0
	}

	line := w.lines[w.cursorRow]
	if w.cursorCol > len(line) {
		w.cursorCol = len(line)
	}

	w.handleSelection(mods, startSel)
	w.notifyCursorChanged()
}

func (w *EditableRichTextWidget) moveCursorPageDown(mods fyne.KeyModifier) {
	startSel := w.hasSelection()

	linesPerPage := 20 // TODO: вычислить из viewport size
	w.cursorRow += linesPerPage
	if w.cursorRow >= len(w.lines) {
		w.cursorRow = len(w.lines) - 1
	}

	line := w.lines[w.cursorRow]
	if w.cursorCol > len(line) {
		w.cursorCol = len(line)
	}

	w.handleSelection(mods, startSel)
	w.notifyCursorChanged()
}

// Выделение текста

func (w *EditableRichTextWidget) handleSelection(mods fyne.KeyModifier, hadSelection bool) {
	if mods&fyne.KeyModifierShift != 0 {
		if !hadSelection {
			w.selectionStart = TextPosition{Row: w.cursorRow, Col: w.cursorCol}
		}
		w.selectionEnd = TextPosition{Row: w.cursorRow, Col: w.cursorCol}
		w.selecting = true
	} else {
		w.clearSelection()
	}
}

func (w *EditableRichTextWidget) hasSelection() bool {
	return w.selectionStart != w.selectionEnd
}

func (w *EditableRichTextWidget) clearSelection() {
	w.selectionStart = TextPosition{}
	w.selectionEnd = TextPosition{}
	w.selecting = false
}

func (w *EditableRichTextWidget) deleteSelection() {
	if !w.hasSelection() {
		return
	}

	start, end := w.normalizeSelection()

	// Удаляем выделенный текст
	if start.Row == end.Row {
		// Одна строка
		line := w.lines[start.Row]
		w.lines[start.Row] = line[:start.Col] + line[end.Col:]
	} else {
		// Несколько строк
		firstLine := w.lines[start.Row][:start.Col]
		lastLine := w.lines[end.Row][end.Col:]
		w.lines[start.Row] = firstLine + lastLine
		w.lines = append(w.lines[:start.Row+1], w.lines[end.Row+1:]...)
	}

	w.cursorRow = start.Row
	w.cursorCol = start.Col
	w.clearSelection()
	w.updateTextFromLines()
}

func (w *EditableRichTextWidget) indentSelection(indent bool) {
	start, end := w.normalizeSelection()

	for row := start.Row; row <= end.Row; row++ {
		line := w.lines[row]
		if indent {
			w.lines[row] = strings.Repeat(" ", w.tabSize) + line
		} else {
			// Удаляем отступ
			trimmed := strings.TrimLeft(line, " \t")
			removed := len(line) - len(trimmed)
			if removed > w.tabSize {
				removed = w.tabSize
			}
			w.lines[row] = line[removed:]
		}
	}

	w.updateTextFromLines()
}

func (w *EditableRichTextWidget) normalizeSelection() (TextPosition, TextPosition) {
	start := w.selectionStart
	end := w.selectionEnd

	if start.Row > end.Row || (start.Row == end.Row && start.Col > end.Col) {
		start, end = end, start
	}

	return start, end
}

// Обновление текста

func (w *EditableRichTextWidget) updateTextFromLines() {
	w.text = strings.Join(w.lines, "\n")
	w.clearRenderCache()
	w.applySyntaxHighlighting()
}

func (w *EditableRichTextWidget) notifyChanged() {
	fyne.Do(func() {
		w.Refresh()
		if w.onChanged != nil {
			w.onChanged(w.text)
		}
	})
}

func (w *EditableRichTextWidget) notifyCursorChanged() {
	fyne.Do(func() {
		w.Refresh()
		if w.onCursorChanged != nil {
			w.onCursorChanged(w.cursorRow, w.cursorCol)
		}
	})
}

// Курсор анимация

func (w *EditableRichTextWidget) startCursorBlink() {
	w.cursorBlink = time.NewTicker(500 * time.Millisecond)
	go func() {
		for {
			select {
			case <-w.cursorBlink.C:
				w.cursorVisible = !w.cursorVisible
				fyne.Do(func() {
					w.Refresh()
				})
			case <-w.cursorStop:
				w.cursorBlink.Stop()
				return
			}
		}
	}()
}

func (w *EditableRichTextWidget) stopCursorBlink() {
	select {
	case w.cursorStop <- true:
	default:
	}
}

// Кэширование

func (w *EditableRichTextWidget) clearRenderCache() {
	w.renderCache = make(map[int]*renderedLine)
}

// Фокус

func (w *EditableRichTextWidget) FocusGained() {
	w.focused = true
	w.cursorVisible = true
	w.Refresh()
}

func (w *EditableRichTextWidget) FocusLost() {
	w.focused = false
	w.cursorVisible = false
	w.Refresh()
}

func (w *EditableRichTextWidget) Focused() bool {
	return w.focused
}

// Интерфейсы

var _ fyne.Focusable = (*EditableRichTextWidget)(nil)
var _ fyne.Tappable = (*EditableRichTextWidget)(nil)
var _ fyne.Draggable = (*EditableRichTextWidget)(nil)
var _ desktop.Keyable = (*EditableRichTextWidget)(nil)

// Tapped обрабатывает клик
func (w *EditableRichTextWidget) Tapped(event *fyne.PointEvent) {
	w.focused = true
	// TODO: установить позицию курсора по координатам клика
}

// Dragged обрабатывает перетаскивание (выделение)
func (w *EditableRichTextWidget) Dragged(event *fyne.DragEvent) {
	// TODO: обновить выделение
}

// DragEnd завершает перетаскивание
func (w *EditableRichTextWidget) DragEnd() {
	w.selecting = false
}

// MinSize возвращает минимальный размер
func (w *EditableRichTextWidget) MinSize() fyne.Size {
	w.mutex.RLock()
	defer w.mutex.RUnlock()

	height := w.lineHeight * float32(len(w.lines))
	width := float32(800) // Default width

	return fyne.NewSize(width, height)
}

// Cleanup освобождает ресурсы
func (w *EditableRichTextWidget) Cleanup() {
	w.stopCursorBlink()

	w.mutex.Lock()
	defer w.mutex.Unlock()

	w.renderCache = make(map[int]*renderedLine)
	w.syntaxCache = make(map[string][]chroma.Token)
}

// GetSelectedText возвращает выделенный текст
func (w *EditableRichTextWidget) GetSelectedText() string {
	w.mutex.RLock()
	defer w.mutex.RUnlock()

	if !w.selecting {
		return ""
	}

	start := w.selectionStart
	end := w.selectionEnd

	// Нормализуем порядок (start должен быть раньше end)
	if start.Row > end.Row || (start.Row == end.Row && start.Col > end.Col) {
		start, end = end, start
	}

	// Одна строка
	if start.Row == end.Row {
		if start.Row < 0 || start.Row >= len(w.lines) {
			return ""
		}
		line := w.lines[start.Row]
		if start.Col < 0 {
			start.Col = 0
		}
		if end.Col > len(line) {
			end.Col = len(line)
		}
		if start.Col >= end.Col {
			return ""
		}
		return line[start.Col:end.Col]
	}

	// Несколько строк
	var result strings.Builder
	for row := start.Row; row <= end.Row; row++ {
		if row < 0 || row >= len(w.lines) {
			continue
		}
		line := w.lines[row]
		if row == start.Row {
			// Первая строка - от start.Col до конца
			if start.Col < len(line) {
				result.WriteString(line[start.Col:])
			}
		} else if row == end.Row {
			// Последняя строка - от начала до end.Col
			if end.Col > 0 && end.Col <= len(line) {
				result.WriteString(line[:end.Col])
			}
		} else {
			// Средние строки - полностью
			result.WriteString(line)
		}
		if row < end.Row {
			result.WriteRune('\n')
		}
	}
	return result.String()
}

// SelectAll выделяет весь текст
func (w *EditableRichTextWidget) SelectAll() {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	w.selectionStart = TextPosition{Row: 0, Col: 0}
	if len(w.lines) > 0 {
		lastRow := len(w.lines) - 1
		w.selectionEnd = TextPosition{Row: lastRow, Col: len(w.lines[lastRow])}
	} else {
		w.selectionEnd = TextPosition{Row: 0, Col: 0}
	}
	w.selecting = true
	w.Refresh()
}

// TypedShortcut обрабатывает горячие клавиши (Cut, Copy, Paste, SelectAll)
func (w *EditableRichTextWidget) TypedShortcut(shortcut fyne.Shortcut) {
	switch sc := shortcut.(type) {
	case *fyne.ShortcutCut:
		// Cut - копируем выделенное в буфер и удаляем
		selected := w.GetSelectedText()
		if selected != "" && sc.Clipboard != nil {
			sc.Clipboard.SetContent(selected)
			// TODO: Удалить выделенный текст
			w.selecting = false
			w.Refresh()
		}
	case *fyne.ShortcutCopy:
		// Copy - копируем выделенное в буфер
		selected := w.GetSelectedText()
		if selected != "" && sc.Clipboard != nil {
			sc.Clipboard.SetContent(selected)
		}
	case *fyne.ShortcutPaste:
		// Paste - вставляем из буфера
		if sc.Clipboard != nil {
			_ = sc.Clipboard.Content()
			// TODO: Вставить content в текущую позицию курсора
			w.Refresh()
		}
	case *fyne.ShortcutSelectAll:
		// Select All - выделяем весь текст
		w.SelectAll()
	}
}

// editableRichTextRenderer рендерер для EditableRichTextWidget
type editableRichTextRenderer struct {
	widget  *EditableRichTextWidget
	objects []fyne.CanvasObject
}

func (r *editableRichTextRenderer) Layout(size fyne.Size) {
	// TODO: Полная реализация рендеринга с виртуализацией
	r.objects = []fyne.CanvasObject{}
}

func (r *editableRichTextRenderer) MinSize() fyne.Size {
	return r.widget.MinSize()
}

func (r *editableRichTextRenderer) Refresh() {
	r.Layout(r.widget.Size())
	canvas.Refresh(r.widget)
}

func (r *editableRichTextRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *editableRichTextRenderer) Destroy() {
	r.objects = nil
}
