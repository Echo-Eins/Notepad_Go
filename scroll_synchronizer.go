package main

import (
	"fyne.io/fyne/v2"
	"sync"
	"time"
)

// ScrollEvent описывает событие прокрутки
type ScrollEvent struct {
	Offset    fyne.Position
	Delta     fyne.Delta
	Source    ScrollSource
	Timestamp time.Time
}

// ScrollSource источник события прокрутки
type ScrollSource int

const (
	ScrollSourceScrollbar ScrollSource = iota
	ScrollSourceWheel
	ScrollSourceKeyboard
	ScrollSourceProgrammatic
	ScrollSourceTouch
)

// ScrollObserver интерфейс для наблюдателей за событиями прокрутки
type ScrollObserver interface {
	OnScrollChanged(event ScrollEvent)
	GetObserverID() string
}

// ScrollSynchronizer синхронизирует прокрутку между компонентами
// Реализует паттерн Observer для эффективной синхронизации
type ScrollSynchronizer struct {
	observers map[string]ScrollObserver
	mutex     sync.RWMutex

	// Текущее состояние прокрутки
	currentOffset fyne.Position
	maxOffset     fyne.Position
	viewportSize  fyne.Size

	// Параметры плавной прокрутки
	smoothScrolling     bool
	smoothScrollSpeed   float32
	momentumEnabled     bool
	momentumDecay       float32
	currentMomentum     fyne.Delta
	momentumTimer       *time.Timer
	momentumStopChannel chan bool

	// Дебаунсинг для предотвращения избыточных обновлений
	debounceDelay   time.Duration
	lastUpdateTime  time.Time
	pendingUpdate   *ScrollEvent
	updateTimer     *time.Timer
	updateChannel   chan ScrollEvent
	stopChannel     chan bool
	running         bool

	// Статистика для отладки и оптимизации
	eventCount      int64
	lastEventTime   time.Time
	averageInterval time.Duration
}

// NewScrollSynchronizer создает новый синхронизатор прокрутки
func NewScrollSynchronizer() *ScrollSynchronizer {
	sync := &ScrollSynchronizer{
		observers:           make(map[string]ScrollObserver),
		currentOffset:       fyne.NewPos(0, 0),
		maxOffset:           fyne.NewPos(0, 0),
		smoothScrolling:     true,
		smoothScrollSpeed:   0.15,
		momentumEnabled:     true,
		momentumDecay:       0.95,
		debounceDelay:       5 * time.Millisecond,
		updateChannel:       make(chan ScrollEvent, 100),
		stopChannel:         make(chan bool, 1),
		momentumStopChannel: make(chan bool, 1),
		running:             true,
	}

	// Запускаем воркер для обработки событий
	go sync.eventProcessor()

	return sync
}

// RegisterObserver регистрирует наблюдателя
func (s *ScrollSynchronizer) RegisterObserver(observer ScrollObserver) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	id := observer.GetObserverID()
	s.observers[id] = observer
}

// UnregisterObserver удаляет наблюдателя
func (s *ScrollSynchronizer) UnregisterObserver(observerID string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	delete(s.observers, observerID)
}

// ScrollTo устанавливает позицию прокрутки
func (s *ScrollSynchronizer) ScrollTo(offset fyne.Position, source ScrollSource) {
	// Ограничиваем offset допустимыми значениями
	offset = s.clampOffset(offset)

	event := ScrollEvent{
		Offset:    offset,
		Delta:     fyne.NewDelta(offset.X-s.currentOffset.X, offset.Y-s.currentOffset.Y),
		Source:    source,
		Timestamp: time.Now(),
	}

	// Отправляем событие в канал для обработки
	select {
	case s.updateChannel <- event:
	default:
		// Канал переполнен, пропускаем событие
	}
}

// ScrollBy прокручивает на заданную дельту
func (s *ScrollSynchronizer) ScrollBy(delta fyne.Delta, source ScrollSource) {
	newOffset := fyne.NewPos(
		s.currentOffset.X+delta.DX,
		s.currentOffset.Y+delta.DY,
	)
	s.ScrollTo(newOffset, source)
}

// ScrollByWheel обрабатывает прокрутку колесиком мыши с momentum
func (s *ScrollSynchronizer) ScrollByWheel(delta fyne.Delta) {
	if s.momentumEnabled {
		// Добавляем импульс к текущему моментуму
		s.currentMomentum.DX += delta.DX
		s.currentMomentum.DY += delta.DY

		// Запускаем анимацию моментума если еще не запущена
		if s.momentumTimer == nil {
			go s.animateMomentum()
		}
	} else {
		// Прямая прокрутка без моментума
		s.ScrollBy(delta, ScrollSourceWheel)
	}
}

// animateMomentum анимирует инерционную прокрутку
func (s *ScrollSynchronizer) animateMomentum() {
	ticker := time.NewTicker(16 * time.Millisecond) // ~60 FPS
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Проверяем, есть ли еще импульс
			if abs(s.currentMomentum.DX) < 0.1 && abs(s.currentMomentum.DY) < 0.1 {
				s.currentMomentum = fyne.NewDelta(0, 0)
				s.momentumTimer = nil
				return
			}

			// Применяем текущий импульс
			s.ScrollBy(s.currentMomentum, ScrollSourceWheel)

			// Уменьшаем импульс
			s.currentMomentum.DX *= s.momentumDecay
			s.currentMomentum.DY *= s.momentumDecay

		case <-s.momentumStopChannel:
			s.currentMomentum = fyne.NewDelta(0, 0)
			s.momentumTimer = nil
			return
		}
	}
}

// StopMomentum останавливает инерционную прокрутку
func (s *ScrollSynchronizer) StopMomentum() {
	if s.momentumTimer != nil {
		select {
		case s.momentumStopChannel <- true:
		default:
		}
	}
}

// eventProcessor обрабатывает события прокрутки с дебаунсингом
func (s *ScrollSynchronizer) eventProcessor() {
	for {
		select {
		case event := <-s.updateChannel:
			// Обновляем статистику
			now := time.Now()
			if !s.lastEventTime.IsZero() {
				interval := now.Sub(s.lastEventTime)
				if s.averageInterval == 0 {
					s.averageInterval = interval
				} else {
					s.averageInterval = (s.averageInterval + interval) / 2
				}
			}
			s.lastEventTime = now
			s.eventCount++

			// Применяем дебаунсинг
			if now.Sub(s.lastUpdateTime) < s.debounceDelay {
				s.pendingUpdate = &event
				continue
			}

			// Обрабатываем событие немедленно
			s.processScrollEvent(event)
			s.lastUpdateTime = now
			s.pendingUpdate = nil

		case <-s.stopChannel:
			return
		}
	}
}

// processScrollEvent обрабатывает событие прокрутки
func (s *ScrollSynchronizer) processScrollEvent(event ScrollEvent) {
	s.mutex.Lock()
	s.currentOffset = event.Offset
	s.mutex.Unlock()

	// Уведомляем всех наблюдателей
	s.mutex.RLock()
	observers := make([]ScrollObserver, 0, len(s.observers))
	for _, observer := range s.observers {
		observers = append(observers, observer)
	}
	s.mutex.RUnlock()

	// Отправляем уведомления без удержания блокировки
	for _, observer := range observers {
		observer.OnScrollChanged(event)
	}
}

// GetCurrentOffset возвращает текущую позицию прокрутки
func (s *ScrollSynchronizer) GetCurrentOffset() fyne.Position {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.currentOffset
}

// SetMaxOffset устанавливает максимальную позицию прокрутки
func (s *ScrollSynchronizer) SetMaxOffset(maxOffset fyne.Position) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.maxOffset = maxOffset
}

// SetViewportSize устанавливает размер видимой области
func (s *ScrollSynchronizer) SetViewportSize(size fyne.Size) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.viewportSize = size
}

// clampOffset ограничивает offset допустимыми значениями
func (s *ScrollSynchronizer) clampOffset(offset fyne.Position) fyne.Position {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	x := offset.X
	y := offset.Y

	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x > s.maxOffset.X {
		x = s.maxOffset.X
	}
	if y > s.maxOffset.Y {
		y = s.maxOffset.Y
	}

	return fyne.NewPos(x, y)
}

// EnableSmoothScrolling включает/выключает плавную прокрутку
func (s *ScrollSynchronizer) EnableSmoothScrolling(enabled bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.smoothScrolling = enabled
}

// EnableMomentum включает/выключает инерционную прокрутку
func (s *ScrollSynchronizer) EnableMomentum(enabled bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.momentumEnabled = enabled
	if !enabled {
		s.StopMomentum()
	}
}

// SetSmoothScrollSpeed устанавливает скорость плавной прокрутки (0.0 - 1.0)
func (s *ScrollSynchronizer) SetSmoothScrollSpeed(speed float32) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if speed < 0 {
		speed = 0
	}
	if speed > 1 {
		speed = 1
	}
	s.smoothScrollSpeed = speed
}

// SetMomentumDecay устанавливает коэффициент затухания импульса (0.0 - 1.0)
func (s *ScrollSynchronizer) SetMomentumDecay(decay float32) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if decay < 0 {
		decay = 0
	}
	if decay > 1 {
		decay = 1
	}
	s.momentumDecay = decay
}

// GetStatistics возвращает статистику работы синхронизатора
func (s *ScrollSynchronizer) GetStatistics() ScrollStatistics {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return ScrollStatistics{
		EventCount:      s.eventCount,
		AverageInterval: s.averageInterval,
		ObserverCount:   len(s.observers),
		LastEventTime:   s.lastEventTime,
	}
}

// ScrollStatistics статистика работы синхронизатора
type ScrollStatistics struct {
	EventCount      int64
	AverageInterval time.Duration
	ObserverCount   int
	LastEventTime   time.Time
}

// Shutdown корректно завершает работу синхронизатора
func (s *ScrollSynchronizer) Shutdown() {
	s.mutex.Lock()
	if !s.running {
		s.mutex.Unlock()
		return
	}
	s.running = false
	s.mutex.Unlock()

	// Останавливаем воркер
	select {
	case s.stopChannel <- true:
	default:
	}

	// Останавливаем моментум
	s.StopMomentum()

	// Очищаем наблюдателей
	s.mutex.Lock()
	s.observers = make(map[string]ScrollObserver)
	s.mutex.Unlock()
}

// abs возвращает абсолютное значение
func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
