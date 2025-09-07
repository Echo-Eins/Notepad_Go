package main

import (
	"fmt"
	"image/color"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/alecthomas/chroma/v2"
)

// MinimapWidget - миниатюрная карта кода как в VS Code
type MinimapWidget struct {
	widget.BaseWidget

	// UI компоненты
	mainContainer   *fyne.Container
	scrollContainer *container.Scroll
	canvas          *fyne.Container
	viewport        *ViewportIndicator

	// Связанный редактор
	editor *EditorWidget

	// Состояние
	isVisible bool
	isEnabled bool
	width     float32
	height    float32

	// Содержимое
	content    string
	lines      []string
	totalLines int

	// Отображение
	lineHeight      float32
	charWidth       float32
	fontSize        float32
	maxCharsPerLine int

	// Viewport состояние
	viewportTop    float32
	viewportHeight float32
	visibleLines   int
	scrollPosition float32

	// Подсветка синтаксиса
	syntaxTokens []chroma.Token
	coloredLines []*MinimapLine

	// Кэширование
	renderCache  map[string]*canvas.Rectangle
	lineCache    map[int]*MinimapLine
	cacheMutex   sync.RWMutex
	lastCacheKey string

	// Настройки
	showSyntax      bool
	showLineNumbers bool
	smoothScrolling bool
	autoHide        bool

	// Производительность
	renderMutex sync.RWMutex
	needsRedraw bool
	isRendering bool
	updateChan  chan MinimapUpdate

	// Цвета
	colors MinimapColors

	// Callbacks
	onScrollTo  func(float32)
	onLineClick func(int)
}

// ViewportIndicator - индикатор видимой области в редакторе
type ViewportIndicator struct {
	widget.BaseWidget

	// Позиция и размер
	x, y          float32
	width, height float32

	// Состояние
	isVisible  bool
	isDragging bool
	dragStartY float32

	// Цвета
	backgroundColor color.Color
	borderColor     color.Color
	opacity         uint8

	// Parent minimap
	minimap *MinimapWidget
}

// MinimapLine - представление строки в миниатюре
type MinimapLine struct {
	LineNumber      int
	Content         string
	TrimmedContent  string
	Length          int
	IndentLevel     int
	Segments        []*MinimapSegment
	BackgroundColor color.Color
	IsHighlighted   bool
	IsBookmarked    bool
}

// MinimapSegment - сегмент строки с цветом
type MinimapSegment struct {
	Text      string
	Color     color.Color
	StartCol  int
	EndCol    int
	IsKeyword bool
	IsComment bool
	IsString  bool
}

// MinimapUpdate - обновление для minimap
type MinimapUpdate struct {
	Type           UpdateType
	Content        string
	ScrollPosition float32
	ViewportTop    float32
	ViewportHeight float32
	TriggerRedraw  bool
}

// UpdateType - тип обновления
type UpdateType int

const (
	UpdateContent UpdateType = iota
	UpdateScroll
	UpdateViewport
	UpdateSyntax
)

// MinimapColors - цветовая схема minimap
type MinimapColors struct {
	Background     color.Color
	Text           color.Color
	Comment        color.Color
	String         color.Color
	Keyword        color.Color
	Number         color.Color
	Function       color.Color
	Viewport       color.Color
	ViewportBorder color.Color
	ScrollBar      color.Color
	LineNumber     color.Color
	Selection      color.Color
}

// MinimapColorProvider описывает объекты,
// способные предоставлять цветовую схему для миникарты.
type MinimapColorProvider interface {
	MinimapColors() MinimapColors
}

// MinimapRenderer - кастомный renderer для minimap
type MinimapRenderer struct {
	minimap *MinimapWidget
	objects []fyne.CanvasObject
}

// ViewportRenderer - renderer для viewport indicator
type ViewportRenderer struct {
	viewport *ViewportIndicator
	rect     *canvas.Rectangle
	border   *canvas.Rectangle
}

// NewMinimap создает новую миниатюрную карту кода
func NewMinimap(editor *EditorWidget) *MinimapWidget {
	minimap := &MinimapWidget{
		editor:          editor,
		width:           120, // По умолчанию как в VS Code
		height:          600,
		isVisible:       true,
		isEnabled:       true,
		lineHeight:      2.0, // Очень маленькая высота строки
		charWidth:       1.0, // Очень маленькая ширина символа
		fontSize:        1.0, // Минимальный размер шрифта
		maxCharsPerLine: 100,
		showSyntax:      true,
		showLineNumbers: false,
		smoothScrolling: true,
		autoHide:        false,
		renderCache:     make(map[string]*canvas.Rectangle),
		lineCache:       make(map[int]*MinimapLine),
		updateChan:      make(chan MinimapUpdate, 100),
	}

	minimap.ExtendBaseWidget(minimap)
	minimap.SetupColors()
	minimap.setupComponents()
	minimap.startUpdateWorker()

	return minimap
}

// SetupColors настраивает цветовую схему
func (m *MinimapWidget) SetupColors() {
	// 1) Если редактор реализует MinimapColorProvider — берём палитру оттуда.
	if m.editor != nil {
		if cp, ok := any(m.editor).(MinimapColorProvider); ok {
			m.colors = cp.MinimapColors()
			return
		}
	}

	// 2) Фолбэк-цвета (если редактор не предоставляет палитру)
	m.colors = MinimapColors{
		Background:     color.NRGBA{0x1E, 0x1E, 0x1E, 0xFF},
		Text:           color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF},
		Comment:        color.NRGBA{0x6A, 0x99, 0x55, 0xFF},
		String:         color.NRGBA{0xCE, 0x91, 0x78, 0xFF},
		Keyword:        color.NRGBA{0x56, 0x9C, 0xD6, 0xFF},
		Number:         color.NRGBA{0xB5, 0xCE, 0xA8, 0xFF},
		Function:       color.NRGBA{0xDC, 0xDC, 0xAA, 0xFF},
		Viewport:       color.NRGBA{0x00, 0x78, 0xD4, 0x40}, // полупрозрачный синий
		ViewportBorder: color.NRGBA{0x00, 0x78, 0xD4, 0xFF}, // синий
		ScrollBar:      color.NRGBA{0x5A, 0x5A, 0x5A, 0xFF},
		LineNumber:     color.NRGBA{0x86, 0x86, 0x86, 0xFF},
		Selection:      color.NRGBA{0x26, 0x4F, 0x78, 0x80},
	}
}

// setupComponents создает UI компоненты
func (m *MinimapWidget) setupComponents() {
	// Создаем основной canvas
	m.canvas = container.NewWithoutLayout()

	// Создаем viewport indicator
	m.viewport = NewViewportIndicator(m)

	// Scroll container для больших файлов
	m.scrollContainer = container.NewScroll(m.canvas)
	m.scrollContainer.SetMinSize(fyne.NewSize(m.width, m.height))

	// Отключаем горизонтальную прокрутку
	m.scrollContainer.OnScrolled = func(pos fyne.Position) {
		// Синхронизируем с редактором при прокрутке
		if m.onScrollTo != nil && !m.isRendering {
			relativePos := pos.Y / (m.getContentHeight() - m.height)
			m.onScrollTo(relativePos)
		}
	}

	// Основной контейнер
	m.mainContainer = container.NewBorder(nil, nil, nil, nil, m.scrollContainer)
}

// startUpdateWorker запускает воркер обновлений
func (m *MinimapWidget) startUpdateWorker() {
	go func() {
		ticker := time.NewTicker(16 * time.Millisecond) // ~60 FPS
		defer ticker.Stop()

		for {
			select {
			case update := <-m.updateChan:
				m.handleUpdate(update)

			case <-ticker.C:
				if m.needsRedraw && !m.isRendering {
					m.redrawMinimap()
				}
			}
		}
	}()
}

// handleUpdate обрабатывает обновление
func (m *MinimapWidget) handleUpdate(update MinimapUpdate) {
	switch update.Type {
	case UpdateContent:
		m.setContent(update.Content)
	case UpdateScroll:
		m.updateScrollPosition(update.ScrollPosition)
	case UpdateViewport:
		m.updateViewport(update.ViewportTop, update.ViewportHeight)
	case UpdateSyntax:
		m.updateSyntaxHighlighting()
	}

	if update.TriggerRedraw {
		m.needsRedraw = true
	}
}

// SetContent устанавливает содержимое для отображения
func (m *MinimapWidget) SetContent(content string) {
	update := MinimapUpdate{
		Type:          UpdateContent,
		Content:       content,
		TriggerRedraw: true,
	}

	select {
	case m.updateChan <- update:
	default:
		// Канал заполнен, обновляем сразу
		m.handleUpdate(update)
	}
}

// setContent внутренний метод установки содержимого
func (m *MinimapWidget) setContent(content string) {
	m.renderMutex.Lock()
	defer m.renderMutex.Unlock()

	if m.content == content {
		return
	}

	m.content = content
	m.lines = strings.Split(content, "\n")
	m.totalLines = len(m.lines)

	// Очищаем кэши
	m.clearCaches()

	// Обрабатываем содержимое
	m.processContent()

	// Обновляем подсветку синтаксиса
	if m.showSyntax && m.editor != nil {
		m.applySyntaxHighlighting()
	}
}

// processContent обрабатывает содержимое для отображения
func (m *MinimapWidget) processContent() {
	m.coloredLines = make([]*MinimapLine, len(m.lines))

	for i, line := range m.lines {
		// Обрезаем слишком длинные строки
		trimmed := line
		if len(line) > m.maxCharsPerLine {
			trimmed = line[:m.maxCharsPerLine] + "..."
		}

		minimapLine := &MinimapLine{
			LineNumber:     i + 1,
			Content:        line,
			TrimmedContent: trimmed,
			Length:         len(line),
			IndentLevel:    m.getIndentLevel(line),
			Segments:       []*MinimapSegment{},
		}

		// Создаем базовый сегмент
		if trimmed != "" {
			segment := &MinimapSegment{
				Text:     trimmed,
				Color:    m.colors.Text,
				StartCol: 0,
				EndCol:   len(trimmed),
			}
			minimapLine.Segments = append(minimapLine.Segments, segment)
		}

		m.coloredLines[i] = minimapLine
		m.lineCache[i] = minimapLine
	}
}

// applySyntaxHighlighting применяет подсветку синтаксиса
func (m *MinimapWidget) applySyntaxHighlighting() {
	if m.editor == nil || m.editor.syntaxTokens == nil {
		return
	}

	// Получаем токены из редактора
	m.syntaxTokens = m.editor.syntaxTokens

	// Применяем цвета к сегментам
	m.applyTokensToMinimap()
}

// applyTokensToMinimap применяет токены к миниатюре
func (m *MinimapWidget) applyTokensToMinimap() {
	if len(m.syntaxTokens) == 0 || len(m.coloredLines) == 0 {
		return
	}

	currentLine := 0
	currentCol := 0

	for _, token := range m.syntaxTokens {
		if token.Value == "" {
			continue
		}

		// Определяем цвет токена
		tokenColor := m.getTokenColor(token.Type)

		// Обновляем строки где встречается токен
		tokenLines := strings.Split(token.Value, "\n")

		for i, tokenLine := range tokenLines {
			if currentLine >= len(m.coloredLines) {
				break
			}

			if tokenLine != "" {
				// Создаем новый сегмент с цветом
				segment := &MinimapSegment{
					Text:      tokenLine,
					Color:     tokenColor,
					StartCol:  currentCol,
					EndCol:    currentCol + len(tokenLine),
					IsKeyword: token.Type.InCategory(chroma.Keyword) || token.Type.InSubCategory(chroma.Keyword),
					IsComment: token.Type.InCategory(chroma.Comment) || token.Type.InSubCategory(chroma.Comment),
					IsString:  token.Type.InCategory(chroma.String) || token.Type.InSubCategory(chroma.String),
				}

				// Заменяем или добавляем сегмент
				line := m.coloredLines[currentLine]
				if len(line.Segments) == 1 && line.Segments[0].Color == m.colors.Text {
					// Заменяем базовый сегмент
					line.Segments = []*MinimapSegment{segment}
				} else {
					// Добавляем новый сегмент
					line.Segments = append(line.Segments, segment)
				}
			}

			if i < len(tokenLines)-1 {
				currentLine++
				currentCol = 0
			} else {
				currentCol += len(tokenLine)
			}
		}
	}
}

// getTokenColor возвращает цвет для типа токена
func (m *MinimapWidget) getTokenColor(tokenType chroma.TokenType) color.Color {
	switch {
	case tokenType.InCategory(chroma.Keyword) || tokenType.InSubCategory(chroma.Keyword):
		return m.colors.Keyword
	case tokenType.InCategory(chroma.String) || tokenType.InSubCategory(chroma.String):
		return m.colors.String
	case tokenType.InCategory(chroma.Comment) || tokenType.InSubCategory(chroma.Comment):
		return m.colors.Comment
	case tokenType.InCategory(chroma.Number) || tokenType.InSubCategory(chroma.Number):
		return m.colors.Number
	// Имя функции — это подкатегория NameFunction (а не Name.Function)
	case tokenType == chroma.NameFunction || tokenType.InSubCategory(chroma.NameFunction):
		return m.colors.Function
	default:
		return m.colors.Text
	}
}

// UpdateScrollPosition обновляет позицию прокрутки
func (m *MinimapWidget) UpdateScrollPosition(position float32) {
	update := MinimapUpdate{
		Type:           UpdateScroll,
		ScrollPosition: position,
		TriggerRedraw:  true,
	}

	select {
	case m.updateChan <- update:
	default:
		m.updateScrollPosition(position)
	}
}

// updateScrollPosition внутренний метод обновления прокрутки
func (m *MinimapWidget) updateScrollPosition(position float32) {
	m.scrollPosition = position

	// Обновляем viewport
	m.updateViewportPosition()
}

// UpdateViewport обновляет viewport indicator
func (m *MinimapWidget) UpdateViewport(top, height float32) {
	update := MinimapUpdate{
		Type:           UpdateViewport,
		ViewportTop:    top,
		ViewportHeight: height,
		TriggerRedraw:  true,
	}

	select {
	case m.updateChan <- update:
	default:
		m.updateViewport(top, height)
	}
}

// updateViewport внутренний метод обновления viewport
func (m *MinimapWidget) updateViewport(top, height float32) {
	m.viewportTop = top
	m.viewportHeight = height
	m.visibleLines = int(height / m.lineHeight)

	m.updateViewportPosition()
}

// updateViewportPosition обновляет позицию viewport indicator
func (m *MinimapWidget) updateViewportPosition() {
	if m.viewport == nil {
		return
	}

	contentHeight := m.getContentHeight()

	// Рассчитываем позицию viewport
	y := m.scrollPosition * (contentHeight - m.height)
	if y < 0 {
		y = 0
	}
	if y > contentHeight-m.viewportHeight {
		y = contentHeight - m.viewportHeight
	}

	// Обновляем viewport
	m.viewport.updatePosition(0, y, m.width, m.viewportHeight)
}

// redrawMinimap перерисовывает миниатюру
func (m *MinimapWidget) redrawMinimap() {
	if m.isRendering {
		return
	}

	m.isRendering = true
	defer func() {
		m.isRendering = false
		m.needsRedraw = false
	}()

	m.renderMutex.Lock()
	defer m.renderMutex.Unlock()

	// Очищаем canvas
	m.canvas.Objects = []fyne.CanvasObject{}

	// Рисуем фон
	m.drawBackground()

	// Рисуем содержимое
	m.drawContent()

	// Рисуем viewport indicator
	m.drawViewport()

	// Обновляем размер canvas
	m.updateCanvasSize()

	// Refresh
	m.canvas.Refresh()
}

// drawBackground рисует фон миниатюры
func (m *MinimapWidget) drawBackground() {
	bg := canvas.NewRectangle(m.colors.Background)
	bg.Resize(fyne.NewSize(m.width, m.getContentHeight()))
	bg.Move(fyne.NewPos(0, 0))

	m.canvas.Add(bg)
}

// drawContent рисует содержимое кода
func (m *MinimapWidget) drawContent() {
	if len(m.coloredLines) == 0 {
		return
	}

	for i, line := range m.coloredLines {
		y := float32(i) * m.lineHeight

		// Пропускаем строки вне видимой области (оптимизация)
		if y > m.scrollContainer.Offset.Y+m.height+100 || y < m.scrollContainer.Offset.Y-100 {
			continue
		}

		m.drawLine(line, 0, y)
	}
}

// drawLine рисует отдельную строку
func (m *MinimapWidget) drawLine(line *MinimapLine, x, y float32) {
	if line == nil || len(line.Segments) == 0 {
		return
	}

	currentX := x

	// Рисуем номер строки если включено
	if m.showLineNumbers {
		lineNumText := canvas.NewText(fmt.Sprintf("%d", line.LineNumber), m.colors.LineNumber)
		lineNumText.TextSize = m.fontSize
		lineNumText.Move(fyne.NewPos(currentX, y))
		m.canvas.Add(lineNumText)
		currentX += 30 // Отступ для номера строки
	}

	// Рисуем отступы как вертикальные линии
	if line.IndentLevel > 0 {
		for level := 0; level < line.IndentLevel; level++ {
			indentX := currentX + float32(level)*4*m.charWidth
			indentLine := canvas.NewLine(m.colors.LineNumber)
			indentLine.Position1 = fyne.NewPos(indentX, y)
			indentLine.Position2 = fyne.NewPos(indentX, y+m.lineHeight)
			indentLine.StrokeWidth = 0.5
			m.canvas.Add(indentLine)
		}
	}

	// Рисуем сегменты строки
	for _, segment := range line.Segments {
		if segment.Text == "" {
			continue
		}

		// Создаем прямоугольник для каждого символа (pixel perfect)
		segmentWidth := float32(len(segment.Text)) * m.charWidth

		if segmentWidth > 0 {
			// Для очень маленького minimap рисуем прямоугольники вместо текста
			rect := canvas.NewRectangle(segment.Color)
			rect.Resize(fyne.NewSize(segmentWidth, m.lineHeight*0.8)) // 80% высоты для лучшего вида
			rect.Move(fyne.NewPos(currentX, y+m.lineHeight*0.1))      // Небольшой отступ сверху

			m.canvas.Add(rect)
		}

		currentX += float32(segment.EndCol-segment.StartCol) * m.charWidth
	}
}

// drawViewport рисует индикатор видимой области
func (m *MinimapWidget) drawViewport() {
	if m.viewport == nil || !m.viewport.isVisible {
		return
	}

	// Фон viewport
	bg := canvas.NewRectangle(m.colors.Viewport)
	bg.Resize(fyne.NewSize(m.viewport.width, m.viewport.height))
	bg.Move(fyne.NewPos(m.viewport.x, m.viewport.y))

	// Граница viewport
	border := canvas.NewRectangle(color.NRGBA{0, 0, 0, 0}) // Прозрачный фон
	border.StrokeColor = m.colors.ViewportBorder
	border.StrokeWidth = 1
	border.Resize(fyne.NewSize(m.viewport.width, m.viewport.height))
	border.Move(fyne.NewPos(m.viewport.x, m.viewport.y))

	m.canvas.Add(bg)
	m.canvas.Add(border)
}

// Обработка событий мыши

// Tapped обрабатывает клик по minimap
func (m *MinimapWidget) Tapped(event *fyne.PointEvent) {
	if !m.isEnabled {
		return
	}

	// Рассчитываем позицию клика
	relativeY := event.Position.Y + m.scrollContainer.Offset.Y
	lineNumber := int(relativeY / m.lineHeight)

	if lineNumber >= 0 && lineNumber < m.totalLines {
		// Callback для перехода к строке
		if m.onLineClick != nil {
			m.onLineClick(lineNumber)
		}

		// Обновляем scroll position
		scrollPosition := float32(lineNumber) / float32(m.totalLines)
		if m.onScrollTo != nil {
			m.onScrollTo(scrollPosition)
		}
	}
}

// TappedSecondary обрабатывает правый клик (контекстное меню)
func (m *MinimapWidget) TappedSecondary(event *fyne.PointEvent) {
	m.showContextMenu(event.Position)
}

// MouseIn обрабатывает наведение мыши
func (m *MinimapWidget) MouseIn(event *fyne.PointEvent) {
	// Можно добавить hover эффекты
}

// MouseOut обрабатывает уход мыши
func (m *MinimapWidget) MouseOut() {
	// Убираем hover эффекты
}

// MouseMoved обрабатывает движение мыши
func (m *MinimapWidget) MouseMoved(event *fyne.PointEvent) {
	// Можно добавить preview или hover линии
}

// Scrolled обрабатывает прокрутку колесика мыши
func (m *MinimapWidget) Scrolled(event *fyne.ScrollEvent) {
	// Прокручиваем minimap и синхронизируем с редактором
	delta := event.Scrolled.DY

	// Рассчитываем новую позицию
	contentHeight := m.getContentHeight()
	currentOffset := m.scrollContainer.Offset.Y
	newOffset := currentOffset - delta*10 // Множитель для скорости прокрутки

	if newOffset < 0 {
		newOffset = 0
	}
	if newOffset > contentHeight-m.height {
		newOffset = contentHeight - m.height
	}

	// Обновляем scroll position
	if contentHeight > m.height {
		scrollPosition := newOffset / (contentHeight - m.height)
		if m.onScrollTo != nil {
			m.onScrollTo(scrollPosition)
		}
	}
}

// Вспомогательные методы

// getContentHeight возвращает общую высоту содержимого
func (m *MinimapWidget) getContentHeight() float32 {
	return float32(m.totalLines) * m.lineHeight
}

// getIndentLevel возвращает уровень отступа строки
func (m *MinimapWidget) getIndentLevel(line string) int {
	level := 0
	for _, char := range line {
		if char == ' ' {
			level++
		} else if char == '\t' {
			level += 4
		} else {
			break
		}
	}
	return level / 4
}

// clearCaches очищает кэши
func (m *MinimapWidget) clearCaches() {
	m.cacheMutex.Lock()
	defer m.cacheMutex.Unlock()

	m.renderCache = make(map[string]*canvas.Rectangle)
	m.lineCache = make(map[int]*MinimapLine)
	m.lastCacheKey = ""
}

// updateCanvasSize обновляет размер canvas
func (m *MinimapWidget) updateCanvasSize() {
	contentHeight := m.getContentHeight()
	m.canvas.Resize(fyne.NewSize(m.width, contentHeight))
}

// showContextMenu показывает контекстное меню
func (m *MinimapWidget) showContextMenu(pos fyne.Position) {
	// Меню настроек Minimap
	c := fyne.CurrentApp().Driver().CanvasForObject(m.mainContainer)
	if c == nil {
		return
	}

	menu := fyne.NewMenu("",
		fyne.NewMenuItem("Toggle Syntax Highlighting", func() {
			m.SetShowSyntax(!m.showSyntax)
			m.Refresh()
		}),
		fyne.NewMenuItem("Toggle Line Numbers", func() {
			m.SetShowLineNumbers(!m.showLineNumbers)
			m.Refresh()
		}),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Hide Minimap", func() {
			m.SetVisible(false)
		}),
	)

	widget.NewPopUpMenu(menu, c).ShowAtPosition(pos)
}

// Публичные методы для настроек

// SetVisible устанавливает видимость minimap
func (m *MinimapWidget) SetVisible(visible bool) {
	m.isVisible = visible
	if m.mainContainer != nil {
		if visible {
			m.mainContainer.Show()
		} else {
			m.mainContainer.Hide()
		}
	}
}

// IsVisible возвращает состояние видимости
func (m *MinimapWidget) IsVisible() bool {
	return m.isVisible
}

// SetWidth устанавливает ширину minimap
func (m *MinimapWidget) SetWidth(width float32) {
	m.width = width
	if m.scrollContainer != nil {
		m.scrollContainer.Resize(fyne.NewSize(width, m.height))
	}
	m.needsRedraw = true
}

// GetWidth возвращает ширину minimap
func (m *MinimapWidget) GetWidth() float32 {
	return m.width
}

// SetShowSyntax включает/выключает подсветку синтаксиса
func (m *MinimapWidget) SetShowSyntax(show bool) {
	m.showSyntax = show
	m.needsRedraw = true
}

// SetShowLineNumbers включает/выключает номера строк
func (m *MinimapWidget) SetShowLineNumbers(show bool) {
	m.showLineNumbers = show
	m.needsRedraw = true
}

// SetSmoothScrolling включает/выключает плавную прокрутку
func (m *MinimapWidget) SetSmoothScrolling(smooth bool) {
	m.smoothScrolling = smooth
}

// SetCallbacks устанавливает callback функции
func (m *MinimapWidget) SetCallbacks(onScrollTo func(float32), onLineClick func(int)) {
	m.onScrollTo = onScrollTo
	m.onLineClick = onLineClick
}

// Refresh принудительно обновляет minimap
func (m *MinimapWidget) Refresh() {
	m.needsRedraw = true
}

// UpdateSyntaxHighlighting обновляет подсветку синтаксиса
func (m *MinimapWidget) UpdateSyntaxHighlighting() {
	if m.showSyntax {
		update := MinimapUpdate{
			Type:          UpdateSyntax,
			TriggerRedraw: true,
		}

		select {
		case m.updateChan <- update:
		default:
			m.updateSyntaxHighlighting()
		}
	}
}

// updateSyntaxHighlighting внутренний метод обновления подсветки
func (m *MinimapWidget) updateSyntaxHighlighting() {
	m.applySyntaxHighlighting()
	m.needsRedraw = true
}

// CreateRenderer реализует интерфейс fyne.Widget
func (m *MinimapWidget) CreateRenderer() fyne.WidgetRenderer {
	return &MinimapRenderer{minimap: m}
}

// MinimapRenderer methods

// Layout устанавливает размеры и позиции объектов
func (r *MinimapRenderer) Layout(size fyne.Size) {
	if r.minimap.mainContainer != nil {
		r.minimap.mainContainer.Resize(size)
	}
}

// MinSize возвращает минимальный размер
func (r *MinimapRenderer) MinSize() fyne.Size {
	return fyne.NewSize(r.minimap.width, 100)
}

// Objects возвращает объекты для отрисовки
func (r *MinimapRenderer) Objects() []fyne.CanvasObject {
	if r.minimap.mainContainer != nil {
		return []fyne.CanvasObject{r.minimap.mainContainer}
	}
	return []fyne.CanvasObject{}
}

// Refresh обновляет renderer
func (r *MinimapRenderer) Refresh() {
	r.minimap.needsRedraw = true
	if r.minimap.mainContainer != nil {
		r.minimap.mainContainer.Refresh()
	}
}

// Destroy очищает ресурсы renderer
func (r *MinimapRenderer) Destroy() {
	// Очистка если необходима
}

// ViewportIndicator implementation

// NewViewportIndicator создает новый индикатор viewport
func NewViewportIndicator(minimap *MinimapWidget) *ViewportIndicator {
	viewport := &ViewportIndicator{
		minimap:         minimap,
		isVisible:       true,
		backgroundColor: minimap.colors.Viewport,
		borderColor:     minimap.colors.ViewportBorder,
		opacity:         128,
		width:           minimap.width,
		height:          100, // Начальная высота
	}

	viewport.ExtendBaseWidget(viewport)
	return viewport
}

// updatePosition обновляет позицию viewport indicator
func (v *ViewportIndicator) updatePosition(x, y, width, height float32) {
	v.x = x
	v.y = y
	v.width = width
	v.height = height

	v.Refresh()
}

// setVisible устанавливает видимость viewport
func (v *ViewportIndicator) setVisible(visible bool) {
	v.isVisible = visible
	v.Refresh()
}

// CreateRenderer для ViewportIndicator
func (v *ViewportIndicator) CreateRenderer() fyne.WidgetRenderer {
	rect := canvas.NewRectangle(v.backgroundColor)
	border := canvas.NewRectangle(color.NRGBA{0, 0, 0, 0})
	border.StrokeColor = v.borderColor
	border.StrokeWidth = 1

	return &ViewportRenderer{
		viewport: v,
		rect:     rect,
		border:   border,
	}
}

// ViewportRenderer methods

// Layout для ViewportRenderer
func (r *ViewportRenderer) Layout(size fyne.Size) {
	r.rect.Resize(size)
	r.border.Resize(size)
}

// MinSize для ViewportRenderer
func (r *ViewportRenderer) MinSize() fyne.Size {
	return fyne.NewSize(10, 10)
}

// Objects для ViewportRenderer
func (r *ViewportRenderer) Objects() []fyne.CanvasObject {
	if r.viewport.isVisible {
		return []fyne.CanvasObject{r.rect, r.border}
	}
	return []fyne.CanvasObject{}
}

// Refresh для ViewportRenderer
func (r *ViewportRenderer) Refresh() {
	r.rect.FillColor = r.viewport.backgroundColor
	r.border.StrokeColor = r.viewport.borderColor
	r.rect.Refresh()
	r.border.Refresh()
}

// Destroy для ViewportRenderer
func (r *ViewportRenderer) Destroy() {
	// Очистка если необходима
}

// Cleanup очищает ресурсы minimap
func (m *MinimapWidget) Cleanup() {
	// Останавливаем воркеры
	close(m.updateChan)

	// Очищаем кэши
	m.clearCaches()
}
