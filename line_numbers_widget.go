package main

import (
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"image/color"
	"sync"
)

// LineNumbersWidget custom widget для отображения номеров строк
// с виртуализацией и синхронизацией скролла
type LineNumbersWidget struct {
	widget.BaseWidget

	// Данные
	totalLines int
	bookmarks  map[int]bool
	lintErrors map[int]string

	// Визуализация
	lineHeight   float32
	digitCount   int
	width        float32
	scrollOffset float32

	// Виртуализация
	firstVisibleLine int
	lastVisibleLine  int
	bufferLines      int // Дополнительные строки вне видимой области

	// Цвета
	backgroundColor color.Color
	textColor       color.Color
	bookmarkColor   color.Color
	errorColor      color.Color
	currentLineColor color.Color

	// Синхронизация
	scrollSync   *ScrollSynchronizer
	observerID   string
	currentLine  int
	mutex        sync.RWMutex

	// Кэширование рендеринга
	cachedLines  map[int]*canvas.Text
	cacheEnabled bool
	maxCacheSize int

	// Обработчики событий
	onLineClicked func(line int)
}

// NewLineNumbersWidget создает новый виджет номеров строк
func NewLineNumbersWidget(totalLines int, scrollSync *ScrollSynchronizer) *LineNumbersWidget {
	widget := &LineNumbersWidget{
		totalLines:      totalLines,
		bookmarks:       make(map[int]bool),
		lintErrors:      make(map[int]string),
		lineHeight:      theme.TextSize() * 1.2,
		bufferLines:     5, // Рендерим 5 дополнительных строк сверху и снизу
		backgroundColor: theme.BackgroundColor(),
		textColor:       theme.ForegroundColor(),
		bookmarkColor:   color.RGBA{R: 255, G: 215, A: 0, A: 255}, // Gold
		errorColor:      color.RGBA{R: 255, G: 0, B: 0, A: 255},
		currentLineColor: theme.PrimaryColor(),
		scrollSync:      scrollSync,
		observerID:      "line_numbers_widget",
		cachedLines:     make(map[int]*canvas.Text),
		cacheEnabled:    true,
		maxCacheSize:    1000,
	}

	widget.ExtendBaseWidget(widget)
	widget.updateDigitCount()
	widget.calculateWidth()

	// Регистрируем наблюдателя в ScrollSynchronizer
	if scrollSync != nil {
		scrollSync.RegisterObserver(widget)
	}

	return widget
}

// GetObserverID реализует интерфейс ScrollObserver
func (w *LineNumbersWidget) GetObserverID() string {
	return w.observerID
}

// OnScrollChanged реализует интерфейс ScrollObserver
func (w *LineNumbersWidget) OnScrollChanged(event ScrollEvent) {
	w.mutex.Lock()
	w.scrollOffset = event.Offset.Y
	w.updateVisibleRange()
	w.mutex.Unlock()

	w.Refresh()
}

// CreateRenderer создает renderer для виджета
func (w *LineNumbersWidget) CreateRenderer() fyne.WidgetRenderer {
	return &lineNumbersRenderer{
		widget: w,
		objects: []fyne.CanvasObject{},
	}
}

// SetTotalLines устанавливает общее количество строк
func (w *LineNumbersWidget) SetTotalLines(lines int) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if w.totalLines == lines {
		return
	}

	w.totalLines = lines
	w.updateDigitCount()
	w.calculateWidth()
	w.updateVisibleRange()
	w.clearCache()

	fyne.Do(func() {
		w.Refresh()
	})
}

// SetBookmarks устанавливает закладки
func (w *LineNumbersWidget) SetBookmarks(bookmarks []int) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	w.bookmarks = make(map[int]bool)
	for _, line := range bookmarks {
		w.bookmarks[line] = true
	}

	w.clearCache()
	fyne.Do(func() {
		w.Refresh()
	})
}

// SetLintErrors устанавливает ошибки линтера
func (w *LineNumbersWidget) SetLintErrors(errors map[int]string) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	w.lintErrors = errors
	w.clearCache()

	fyne.Do(func() {
		w.Refresh()
	})
}

// SetCurrentLine устанавливает текущую строку
func (w *LineNumbersWidget) SetCurrentLine(line int) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if w.currentLine == line {
		return
	}

	w.currentLine = line
	fyne.Do(func() {
		w.Refresh()
	})
}

// SetOnLineClicked устанавливает обработчик клика по номеру строки
func (w *LineNumbersWidget) SetOnLineClicked(callback func(line int)) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.onLineClicked = callback
}

// Tapped обрабатывает клик по номеру строки
func (w *LineNumbersWidget) Tapped(event *fyne.PointEvent) {
	w.mutex.RLock()
	scrollOffset := w.scrollOffset
	lineHeight := w.lineHeight
	onLineClicked := w.onLineClicked
	w.mutex.RUnlock()

	// Вычисляем номер строки по Y-координате
	adjustedY := event.Position.Y + scrollOffset
	lineNumber := int(adjustedY/lineHeight) + 1

	if lineNumber < 1 {
		lineNumber = 1
	}
	if lineNumber > w.totalLines {
		lineNumber = w.totalLines
	}

	// Вызываем callback
	if onLineClicked != nil {
		onLineClicked(lineNumber)
	}
}

// MinSize возвращает минимальный размер виджета
func (w *LineNumbersWidget) MinSize() fyne.Size {
	w.mutex.RLock()
	defer w.mutex.RUnlock()

	return fyne.NewSize(w.width, w.lineHeight*float32(w.totalLines))
}

// updateDigitCount обновляет количество цифр для форматирования
func (w *LineNumbersWidget) updateDigitCount() {
	w.digitCount = len(fmt.Sprintf("%d", w.totalLines))
	if w.digitCount < 2 {
		w.digitCount = 2
	}
}

// calculateWidth вычисляет ширину виджета
func (w *LineNumbersWidget) calculateWidth() {
	// Ширина = ширина цифр + отступы + место для маркера закладки
	markerWidth := MeasureString("★ ", theme.TextSize()).Width
	digitWidth := MeasureString(fmt.Sprintf("%*d", w.digitCount, 0), theme.TextSize()).Width
	padding := float32(16)

	w.width = markerWidth + digitWidth + padding
}

// updateVisibleRange обновляет диапазон видимых строк
func (w *LineNumbersWidget) updateVisibleRange() {
	// Вычисляем первую видимую строку
	w.firstVisibleLine = int(w.scrollOffset/w.lineHeight) + 1
	if w.firstVisibleLine < 1 {
		w.firstVisibleLine = 1
	}

	// Вычисляем последнюю видимую строку с учетом размера виджета
	size := w.Size()
	visibleLines := int(size.Height / w.lineHeight)
	w.lastVisibleLine = w.firstVisibleLine + visibleLines

	if w.lastVisibleLine > w.totalLines {
		w.lastVisibleLine = w.totalLines
	}

	// Добавляем буферные строки
	w.firstVisibleLine -= w.bufferLines
	if w.firstVisibleLine < 1 {
		w.firstVisibleLine = 1
	}

	w.lastVisibleLine += w.bufferLines
	if w.lastVisibleLine > w.totalLines {
		w.lastVisibleLine = w.totalLines
	}
}

// getLineNumber возвращает отформатированный номер строки
func (w *LineNumbersWidget) getLineNumber(line int) string {
	marker := "  "
	if w.bookmarks[line] {
		marker = "★ "
	}
	return fmt.Sprintf("%s%*d", marker, w.digitCount, line)
}

// getLineColor возвращает цвет для номера строки
func (w *LineNumbersWidget) getLineColor(line int) color.Color {
	if w.lintErrors[line] != "" {
		return w.errorColor
	}
	if line == w.currentLine {
		return w.currentLineColor
	}
	if w.bookmarks[line] {
		return w.bookmarkColor
	}
	return w.textColor
}

// clearCache очищает кэш рендеринга
func (w *LineNumbersWidget) clearCache() {
	if w.cacheEnabled {
		w.cachedLines = make(map[int]*canvas.Text)
	}
}

// getCachedLine возвращает кэшированный объект или создает новый
func (w *LineNumbersWidget) getCachedLine(line int) *canvas.Text {
	if !w.cacheEnabled {
		return nil
	}

	if cached, ok := w.cachedLines[line]; ok {
		return cached
	}

	// Проверяем размер кэша
	if len(w.cachedLines) >= w.maxCacheSize {
		// Простая стратегия: очищаем кэш полностью
		w.cachedLines = make(map[int]*canvas.Text)
	}

	return nil
}

// setCachedLine сохраняет объект в кэш
func (w *LineNumbersWidget) setCachedLine(line int, text *canvas.Text) {
	if w.cacheEnabled && len(w.cachedLines) < w.maxCacheSize {
		w.cachedLines[line] = text
	}
}

// EnableCache включает/выключает кэширование
func (w *LineNumbersWidget) EnableCache(enabled bool) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	w.cacheEnabled = enabled
	if !enabled {
		w.cachedLines = make(map[int]*canvas.Text)
	}
}

// SetMaxCacheSize устанавливает максимальный размер кэша
func (w *LineNumbersWidget) SetMaxCacheSize(size int) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	w.maxCacheSize = size
	if len(w.cachedLines) > size {
		w.cachedLines = make(map[int]*canvas.Text)
	}
}

// Cleanup освобождает ресурсы
func (w *LineNumbersWidget) Cleanup() {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if w.scrollSync != nil {
		w.scrollSync.UnregisterObserver(w.observerID)
		w.scrollSync = nil
	}

	w.cachedLines = make(map[int]*canvas.Text)
	w.bookmarks = make(map[int]bool)
	w.lintErrors = make(map[int]string)
}

// lineNumbersRenderer рендерер для виджета номеров строк
type lineNumbersRenderer struct {
	widget  *LineNumbersWidget
	objects []fyne.CanvasObject
}

// Layout выполняет размещение элементов
func (r *lineNumbersRenderer) Layout(size fyne.Size) {
	// Виртуализация: создаем объекты только для видимых строк
	r.widget.mutex.RLock()
	defer r.widget.mutex.RUnlock()

	r.objects = []fyne.CanvasObject{}

	firstLine := r.widget.firstVisibleLine
	lastLine := r.widget.lastVisibleLine
	lineHeight := r.widget.lineHeight
	scrollOffset := r.widget.scrollOffset

	for line := firstLine; line <= lastLine; line++ {
		// Вычисляем Y-позицию строки
		yPos := float32(line-1)*lineHeight - scrollOffset

		// Создаем или получаем из кэша текстовый объект
		var textObj *canvas.Text
		if cached := r.widget.getCachedLine(line); cached != nil {
			textObj = cached
		} else {
			textObj = canvas.NewText(r.widget.getLineNumber(line), r.widget.getLineColor(line))
			textObj.TextSize = theme.TextSize()
			textObj.TextStyle = fyne.TextStyle{Monospace: true}
			textObj.Alignment = fyne.TextAlignTrailing
			r.widget.setCachedLine(line, textObj)
		}

		// Обновляем позицию и цвет (цвет может измениться)
		textObj.Color = r.widget.getLineColor(line)
		textObj.Move(fyne.NewPos(0, yPos))
		textObj.Resize(fyne.NewSize(size.Width, lineHeight))

		r.objects = append(r.objects, textObj)
	}
}

// MinSize возвращает минимальный размер
func (r *lineNumbersRenderer) MinSize() fyne.Size {
	return r.widget.MinSize()
}

// Refresh обновляет рендерер
func (r *lineNumbersRenderer) Refresh() {
	r.widget.mutex.Lock()
	r.widget.updateVisibleRange()
	r.widget.mutex.Unlock()

	r.Layout(r.widget.Size())
	canvas.Refresh(r.widget)
}

// Objects возвращает список объектов для отрисовки
func (r *lineNumbersRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

// Destroy освобождает ресурсы
func (r *lineNumbersRenderer) Destroy() {
	r.objects = nil
}
