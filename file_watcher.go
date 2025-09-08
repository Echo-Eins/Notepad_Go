package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileWatcher наблюдает за изменениями файлов
type FileWatcher struct {
	watcher       *fsnotify.Watcher
	watchedFiles  map[string]*WatchedFile
	watchedDirs   map[string]bool
	mutex         sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	eventHandlers map[FileEventType][]FileEventHandler
	debounceDelay time.Duration
	pendingEvents map[string]*PendingEvent
	eventMutex    sync.Mutex
}

// WatchedFile представляет наблюдаемый файл
type WatchedFile struct {
	Path         string
	LastModified time.Time
	Size         int64
	Hash         string
	IsDirectory  bool
}

// calculateFileHash вычисляет SHA-256 хеш содержимого файла
func calculateFileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// FileEvent представляет событие файловой системы
type FileEvent struct {
	Type      FileEventType
	Path      string
	OldPath   string // Для переименования
	Timestamp time.Time
	Info      os.FileInfo
}

// FileEventType тип события файла
type FileEventType int

const (
	FileCreated FileEventType = iota
	FileModified
	FileDeleted
	FileRenamed
	FilePermissionChanged
)

// FileEventHandler обработчик событий файлов
type FileEventHandler func(event FileEvent)

// PendingEvent отложенное событие для debouncing
type PendingEvent struct {
	Event FileEvent
	Timer *time.Timer
}

// NewFileWatcher создает новый file watcher
func NewFileWatcher(debounceDelay time.Duration) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	fw := &FileWatcher{
		watcher:       watcher,
		watchedFiles:  make(map[string]*WatchedFile),
		watchedDirs:   make(map[string]bool),
		ctx:           ctx,
		cancel:        cancel,
		eventHandlers: make(map[FileEventType][]FileEventHandler),
		debounceDelay: debounceDelay,
		pendingEvents: make(map[string]*PendingEvent),
	}

	// Запускаем обработчик событий
	go fw.processEvents()

	return fw, nil
}

// WatchFile добавляет файл для наблюдения
func (fw *FileWatcher) WatchFile(path string) error {
	fw.mutex.Lock()
	defer fw.mutex.Unlock()

	// Получаем абсолютный путь
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %v", err)
	}

	// Проверяем существование файла
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("failed to stat file: %v", err)
	}

	// Добавляем в watcher
	if err := fw.watcher.Add(absPath); err != nil {
		return fmt.Errorf("failed to add file to watcher: %v", err)
	}

	// Вычисляем хеш для файлов
	var hash string
	if !info.IsDir() {
		hash, err = calculateFileHash(absPath)
		if err != nil {
			return fmt.Errorf("failed to hash file: %v", err)
		}
	}

	// Сохраняем информацию о файле
	fw.watchedFiles[absPath] = &WatchedFile{
		Path:         absPath,
		LastModified: info.ModTime(),
		Size:         info.Size(),
		Hash:         hash,
		IsDirectory:  info.IsDir(),
	}

	// Если это директория, добавляем в список директорий
	if info.IsDir() {
		fw.watchedDirs[absPath] = true
	}

	return nil
}

// WatchDirectory добавляет директорию для наблюдения
func (fw *FileWatcher) WatchDirectory(path string, recursive bool) error {
	fw.mutex.Lock()
	defer fw.mutex.Unlock()

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %v", err)
	}

	// Проверяем, что это директория
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("failed to stat directory: %v", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", absPath)
	}

	// Добавляем директорию
	if err := fw.addDirectory(absPath); err != nil {
		return err
	}

	// Рекурсивно добавляем поддиректории
	if recursive {
		err := filepath.Walk(absPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() && path != absPath {
				return fw.addDirectory(path)
			}

			return nil
		})

		if err != nil {
			return fmt.Errorf("failed to walk directory tree: %v", err)
		}
	}

	return nil
}

// addDirectory внутренний метод добавления директории
func (fw *FileWatcher) addDirectory(path string) error {
	if err := fw.watcher.Add(path); err != nil {
		return fmt.Errorf("failed to add directory to watcher: %v", err)
	}

	info, _ := os.Stat(path)
	fw.watchedFiles[path] = &WatchedFile{
		Path:         path,
		LastModified: info.ModTime(),
		Size:         info.Size(),
		IsDirectory:  true,
	}

	fw.watchedDirs[path] = true

	return nil
}

// UnwatchFile удаляет файл из наблюдения
func (fw *FileWatcher) UnwatchFile(path string) error {
	fw.mutex.Lock()
	defer fw.mutex.Unlock()

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %v", err)
	}

	// Удаляем из watcher
	if err := fw.watcher.Remove(absPath); err != nil {
		return fmt.Errorf("failed to remove file from watcher: %v", err)
	}

	// Удаляем из карт
	delete(fw.watchedFiles, absPath)
	delete(fw.watchedDirs, absPath)

	return nil
}

// UnwatchAll удаляет все файлы из наблюдения
func (fw *FileWatcher) UnwatchAll() {
	fw.mutex.Lock()
	defer fw.mutex.Unlock()

	for path := range fw.watchedFiles {
		fw.watcher.Remove(path)
	}

	fw.watchedFiles = make(map[string]*WatchedFile)
	fw.watchedDirs = make(map[string]bool)
}

// OnFileEvent регистрирует обработчик события
func (fw *FileWatcher) OnFileEvent(eventType FileEventType, handler FileEventHandler) {
	fw.mutex.Lock()
	defer fw.mutex.Unlock()

	fw.eventHandlers[eventType] = append(fw.eventHandlers[eventType], handler)
}

// OnAnyEvent регистрирует обработчик для всех событий
func (fw *FileWatcher) OnAnyEvent(handler FileEventHandler) {
	fw.mutex.Lock()
	defer fw.mutex.Unlock()

	eventTypes := []FileEventType{
		FileCreated,
		FileModified,
		FileDeleted,
		FileRenamed,
		FilePermissionChanged,
	}

	for _, eventType := range eventTypes {
		fw.eventHandlers[eventType] = append(fw.eventHandlers[eventType], handler)
	}
}

// processEvents обрабатывает события от fsnotify
func (fw *FileWatcher) processEvents() {
	for {
		select {
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}

			fw.handleEvent(event)

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}

			log.Printf("File watcher error: %v", err)

		case <-fw.ctx.Done():
			return
		}
	}
}

// handleEvent обрабатывает событие от fsnotify
func (fw *FileWatcher) handleEvent(event fsnotify.Event) {
	// Определяем тип события
	var eventType FileEventType
	switch {
	case event.Op&fsnotify.Create == fsnotify.Create:
		eventType = FileCreated
		// Если создана директория в наблюдаемой директории, добавляем ее
		if fw.isWatchedDirectory(filepath.Dir(event.Name)) {
			if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
				fw.mutex.Lock()
				fw.addDirectory(event.Name)
				fw.mutex.Unlock()
			}
		}

	case event.Op&fsnotify.Write == fsnotify.Write:
		eventType = FileModified

	case event.Op&fsnotify.Remove == fsnotify.Remove:
		eventType = FileDeleted
		// Удаляем из списка наблюдаемых
		fw.mutex.Lock()
		delete(fw.watchedFiles, event.Name)
		delete(fw.watchedDirs, event.Name)
		fw.mutex.Unlock()

	case event.Op&fsnotify.Rename == fsnotify.Rename:
		eventType = FileRenamed

	case event.Op&fsnotify.Chmod == fsnotify.Chmod:
		eventType = FilePermissionChanged

	default:
		return
	}

	// Получаем информацию о файле
	var info os.FileInfo
	if eventType != FileDeleted {
		var err error
		info, err = os.Stat(event.Name)
		if err != nil {
			// Файл мог быть удален между событием и проверкой
			if os.IsNotExist(err) {
				eventType = FileDeleted
			} else {
				return
			}
		}
	}

	// Создаем событие
	fileEvent := FileEvent{
		Type:      eventType,
		Path:      event.Name,
		Timestamp: time.Now(),
		Info:      info,
	}

	// Применяем debouncing
	fw.debounceEvent(fileEvent)
}

// debounceEvent откладывает обработку события для группировки
func (fw *FileWatcher) debounceEvent(event FileEvent) {
	fw.eventMutex.Lock()
	defer fw.eventMutex.Unlock()

	// Если уже есть отложенное событие для этого файла, отменяем его
	if pending, exists := fw.pendingEvents[event.Path]; exists {
		pending.Timer.Stop()
	}

	// Создаем новое отложенное событие
	timer := time.AfterFunc(fw.debounceDelay, func() {
		fw.dispatchEvent(event)

		fw.eventMutex.Lock()
		delete(fw.pendingEvents, event.Path)
		fw.eventMutex.Unlock()
	})

	fw.pendingEvents[event.Path] = &PendingEvent{
		Event: event,
		Timer: timer,
	}
}

// dispatchEvent отправляет событие обработчикам
func (fw *FileWatcher) dispatchEvent(event FileEvent) {
	fw.mutex.RLock()
	handlers := fw.eventHandlers[event.Type]
	fw.mutex.RUnlock()

	for _, handler := range handlers {
		// Вызываем обработчик в отдельной горутине
		go handler(event)
	}
}

// isWatchedDirectory проверяет, является ли путь наблюдаемой директорией
func (fw *FileWatcher) isWatchedDirectory(path string) bool {
	fw.mutex.RLock()
	defer fw.mutex.RUnlock()

	return fw.watchedDirs[path]
}

// GetWatchedFiles возвращает список наблюдаемых файлов
func (fw *FileWatcher) GetWatchedFiles() []string {
	fw.mutex.RLock()
	defer fw.mutex.RUnlock()

	files := make([]string, 0, len(fw.watchedFiles))
	for path := range fw.watchedFiles {
		files = append(files, path)
	}

	return files
}

// IsWatching проверяет, наблюдается ли файл
func (fw *FileWatcher) IsWatching(path string) bool {
	fw.mutex.RLock()
	defer fw.mutex.RUnlock()

	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	_, exists := fw.watchedFiles[absPath]
	return exists
}

// Stop останавливает file watcher
func (fw *FileWatcher) Stop() {
	fw.cancel()
	fw.watcher.Close()

	// Отменяем все отложенные события
	fw.eventMutex.Lock()
	for _, pending := range fw.pendingEvents {
		pending.Timer.Stop()
	}
	fw.pendingEvents = make(map[string]*PendingEvent)
	fw.eventMutex.Unlock()
}

// FileChangeDetector определяет изменения в файлах
type FileChangeDetector struct {
	checksums map[string]string
	mutex     sync.RWMutex
}

// NewFileChangeDetector создает новый детектор изменений
func NewFileChangeDetector() *FileChangeDetector {
	return &FileChangeDetector{
		checksums: make(map[string]string),
	}
}

// HasFileChanged проверяет, изменился ли файл
func (fcd *FileChangeDetector) HasFileChanged(path string) (bool, error) {
	// calculateChecksum вычисляет SHA-256 контрольную сумму файла
	newChecksum, err := fcd.calculateChecksum(path)
	if err != nil {
		return false, err
	}

	fcd.mutex.Lock()
	defer fcd.mutex.Unlock()

	// Сравниваем с сохраненной контрольной суммой
	oldChecksum, exists := fcd.checksums[path]
	if !exists {
		// Первая проверка файла
		fcd.checksums[path] = newChecksum
		return false, nil
	}

	if oldChecksum != newChecksum {
		// Файл изменился
		fcd.checksums[path] = newChecksum
		return true, nil
	}

	return false, nil
}

// calculateChecksum вычисляет контрольную сумму файла
func (fcd *FileChangeDetector) calculateChecksum(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	if info.IsDir() {
		return fmt.Sprintf("dir_%d", info.ModTime().Unix()), nil
	}

	return calculateFileHash(path)
}

// UpdateChecksum обновляет контрольную сумму файла
func (fcd *FileChangeDetector) UpdateChecksum(path string) error {
	checksum, err := fcd.calculateChecksum(path)
	if err != nil {
		return err
	}

	fcd.mutex.Lock()
	defer fcd.mutex.Unlock()

	fcd.checksums[path] = checksum
	return nil
}

// ClearChecksum удаляет контрольную сумму файла
func (fcd *FileChangeDetector) ClearChecksum(path string) {
	fcd.mutex.Lock()
	defer fcd.mutex.Unlock()

	delete(fcd.checksums, path)
}

// ClearAllChecksums очищает все контрольные суммы
func (fcd *FileChangeDetector) ClearAllChecksums() {
	fcd.mutex.Lock()
	defer fcd.mutex.Unlock()

	fcd.checksums = make(map[string]string)
}

// DirectoryMonitor мониторит изменения в директории
type DirectoryMonitor struct {
	watcher         *FileWatcher
	changeDetector  *FileChangeDetector
	rootPath        string
	includePatterns []string
	excludePatterns []string
	recursive       bool
}

// NewDirectoryMonitor создает новый монитор директории
func NewDirectoryMonitor(rootPath string, recursive bool) (*DirectoryMonitor, error) {
	watcher, err := NewFileWatcher(500 * time.Millisecond)
	if err != nil {
		return nil, err
	}

	dm := &DirectoryMonitor{
		watcher:        watcher,
		changeDetector: NewFileChangeDetector(),
		rootPath:       rootPath,
		recursive:      recursive,
	}

	// Начинаем наблюдение за директорией
	if err := watcher.WatchDirectory(rootPath, recursive); err != nil {
		return nil, err
	}

	return dm, nil
}

// SetIncludePatterns устанавливает паттерны включения
func (dm *DirectoryMonitor) SetIncludePatterns(patterns []string) {
	dm.includePatterns = patterns
}

// SetExcludePatterns устанавливает паттерны исключения
func (dm *DirectoryMonitor) SetExcludePatterns(patterns []string) {
	dm.excludePatterns = patterns
}

// OnChange регистрирует обработчик изменений
func (dm *DirectoryMonitor) OnChange(handler func(path string, changeType FileEventType)) {
	dm.watcher.OnAnyEvent(func(event FileEvent) {
		// Проверяем паттерны
		if !dm.matchesPatterns(event.Path) {
			return
		}

		handler(event.Path, event.Type)
	})
}

// matchesPatterns проверяет, соответствует ли путь паттернам
func (dm *DirectoryMonitor) matchesPatterns(path string) bool {
	// Проверяем паттерны исключения
	for _, pattern := range dm.excludePatterns {
		matched, _ := filepath.Match(pattern, filepath.Base(path))
		if matched {
			return false
		}
	}

	// Если нет паттернов включения, включаем все
	if len(dm.includePatterns) == 0 {
		return true
	}

	// Проверяем паттерны включения
	for _, pattern := range dm.includePatterns {
		matched, _ := filepath.Match(pattern, filepath.Base(path))
		if matched {
			return true
		}
	}

	return false
}

// Stop останавливает монитор
func (dm *DirectoryMonitor) Stop() {
	dm.watcher.Stop()
}

// GetFileEventTypeString возвращает строковое представление типа события
func GetFileEventTypeString(eventType FileEventType) string {
	switch eventType {
	case FileCreated:
		return "created"
	case FileModified:
		return "modified"
	case FileDeleted:
		return "deleted"
	case FileRenamed:
		return "renamed"
	case FilePermissionChanged:
		return "permission_changed"
	default:
		return "unknown"
	}
}
