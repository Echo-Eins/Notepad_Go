package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/BurntSushi/toml"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"time"
)

// ConfigMigrator обрабатывает миграцию конфигурации с JSON на TOML
type ConfigMigrator struct {
	jsonPath   string
	tomlPath   string
	backupPath string
}

// NewConfigMigrator создает новый мигратор конфигурации
func NewConfigMigrator(configDir string) *ConfigMigrator {
	return &ConfigMigrator{
		jsonPath:   filepath.Join(configDir, "config.json"),
		tomlPath:   filepath.Join(configDir, "config.toml"),
		backupPath: filepath.Join(configDir, "config.json.backup"),
	}
}

// NeedsMigration проверяет, требуется ли миграция
func (m *ConfigMigrator) NeedsMigration() bool {
	// Проверяем наличие JSON файла
	if _, err := os.Stat(m.jsonPath); os.IsNotExist(err) {
		return false
	}

	// Проверяем наличие TOML файла
	if _, err := os.Stat(m.tomlPath); os.IsNotExist(err) {
		return true
	}

	// Оба файла существуют, проверяем время модификации
	jsonInfo, err1 := os.Stat(m.jsonPath)
	tomlInfo, err2 := os.Stat(m.tomlPath)

	if err1 != nil || err2 != nil {
		return false
	}

	// Если JSON новее TOML, требуется миграция
	return jsonInfo.ModTime().After(tomlInfo.ModTime())
}

// Migrate выполняет миграцию с JSON на TOML
func (m *ConfigMigrator) Migrate() error {
	// Читаем JSON конфигурацию
	jsonData, err := ioutil.ReadFile(m.jsonPath)
	if err != nil {
		return fmt.Errorf("failed to read JSON config: %v", err)
	}

	// Парсим JSON в структуру Config
	config := DefaultConfig()
	if err := json.Unmarshal(jsonData, config); err != nil {
		return fmt.Errorf("failed to parse JSON config: %v", err)
	}

	// Создаем backup JSON файла
	if err := m.createBackup(); err != nil {
		return fmt.Errorf("failed to create backup: %v", err)
	}

	// Сохраняем в TOML формате
	if err := m.saveTOML(config); err != nil {
		// Если не удалось сохранить TOML, откатываем backup
		m.restoreBackup()
		return fmt.Errorf("failed to save TOML config: %v", err)
	}

	return nil
}

// saveTOML сохраняет конфигурацию в TOML формат
func (m *ConfigMigrator) saveTOML(config *Config) error {
	var buf bytes.Buffer

	// Создаем TOML encoder
	encoder := toml.NewEncoder(&buf)
	encoder.Indent = "  "

	// Кодируем конфигурацию
	if err := encoder.Encode(config); err != nil {
		return err
	}

	// Записываем в файл
	if err := ioutil.WriteFile(m.tomlPath, buf.Bytes(), 0644); err != nil {
		return err
	}

	return nil
}

// createBackup создает backup JSON файла
func (m *ConfigMigrator) createBackup() error {
	data, err := ioutil.ReadFile(m.jsonPath)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(m.backupPath, data, 0644)
}

// restoreBackup восстанавливает JSON файл из backup
func (m *ConfigMigrator) restoreBackup() error {
	data, err := ioutil.ReadFile(m.backupPath)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(m.jsonPath, data, 0644)
}

// RemoveBackup удаляет backup файл
func (m *ConfigMigrator) RemoveBackup() error {
	if _, err := os.Stat(m.backupPath); os.IsNotExist(err) {
		return nil
	}

	return os.Remove(m.backupPath)
}

// TOMLConfigManager менеджер конфигурации для TOML формата
// с оптимизациями: debouncing, diff-based save, async операции
type TOMLConfigManager struct {
	*ConfigManager // Встраиваем базовый менеджер

	// TOML специфичные поля
	tomlPath string

	// Diff-based save
	lastSavedConfig *Config
	configHash      string

	// Debouncing
	debounceDuration time.Duration
	debounceTimer    *time.Timer
	debounceMutex    sync.Mutex

	// Async save queue
	saveQueue      chan *Config
	saveQueueSize  int
	saveInProgress bool
	saveMutex      sync.Mutex
}

// NewTOMLConfigManager создает новый TOML менеджер конфигурации
func NewTOMLConfigManager(configPath string) *TOMLConfigManager {
	if configPath == "" {
		configDir := getConfigDirectory()
		configPath = filepath.Join(configDir, "config.toml")
	}

	baseManager := &ConfigManager{
		configPath:      configPath,
		changeCallbacks: []func(*Config){},
	}

	manager := &TOMLConfigManager{
		ConfigManager:    baseManager,
		tomlPath:         configPath,
		debounceDuration: 300 * time.Millisecond,
		saveQueueSize:    10,
		saveQueue:        make(chan *Config, 10),
	}

	// Запускаем async save worker
	go manager.saveWorker()

	return manager
}

// LoadConfig загружает конфигурацию из TOML файла
func (tm *TOMLConfigManager) LoadConfig() (*Config, error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	// Проверяем миграцию
	configDir := filepath.Dir(tm.tomlPath)
	migrator := NewConfigMigrator(configDir)

	if migrator.NeedsMigration() {
		if err := migrator.Migrate(); err != nil {
			return nil, fmt.Errorf("config migration failed: %v", err)
		}
		// Удаляем backup после успешной миграции
		migrator.RemoveBackup()
	}

	// Если TOML файл не существует, создаем с настройками по умолчанию
	if _, err := os.Stat(tm.tomlPath); os.IsNotExist(err) {
		tm.config = DefaultConfig()
		tm.config.ConfigPath = tm.tomlPath

		// Создаем директорию если не существует
		dir := filepath.Dir(tm.tomlPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("cannot create config directory: %v", err)
		}

		// Сохраняем конфигурацию по умолчанию
		if err := tm.saveTOMLUnsafe(tm.config); err != nil {
			return nil, fmt.Errorf("cannot save default config: %v", err)
		}

		tm.lastSavedConfig = tm.copyConfig(tm.config)
		tm.isLoaded = true
		return tm.config, nil
	}

	// Читаем TOML файл
	data, err := ioutil.ReadFile(tm.tomlPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read config file: %v", err)
	}

	// Парсим TOML
	config := DefaultConfig() // Начинаем с default значений
	if err := toml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("cannot parse TOML config: %v", err)
	}

	config.ConfigPath = tm.tomlPath

	// Валидируем настройки
	if err := tm.validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid config: %v", err)
	}

	// Применяем миграции если версия изменилась
	if err := tm.migrateConfig(config); err != nil {
		return nil, fmt.Errorf("config migration failed: %v", err)
	}

	tm.config = config
	tm.lastSavedConfig = tm.copyConfig(config)
	tm.isLoaded = true

	// Запускаем file watcher
	tm.startWatching()

	return tm.config, nil
}

// SaveConfig сохраняет конфигурацию с debouncing
func (tm *TOMLConfigManager) SaveConfig() error {
	tm.debounceMutex.Lock()
	defer tm.debounceMutex.Unlock()

	// Отменяем предыдущий таймер
	if tm.debounceTimer != nil {
		tm.debounceTimer.Stop()
	}

	// Создаем новый таймер для debouncing
	tm.debounceTimer = time.AfterFunc(tm.debounceDuration, func() {
		tm.mutex.RLock()
		config := tm.copyConfig(tm.config)
		tm.mutex.RUnlock()

		// Отправляем в очередь для async сохранения
		select {
		case tm.saveQueue <- config:
		default:
			// Очередь полна, логируем ошибку
			fmt.Println("Warning: save queue is full, config save skipped")
		}
	})

	return nil
}

// SaveConfigSync сохраняет конфигурацию синхронно (без debouncing)
func (tm *TOMLConfigManager) SaveConfigSync() error {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	return tm.saveTOMLUnsafe(tm.config)
}

// saveTOMLUnsafe сохраняет конфигурацию в TOML (без блокировки)
func (tm *TOMLConfigManager) saveTOMLUnsafe(config *Config) error {
	if config == nil {
		return fmt.Errorf("config is nil")
	}

	// Валидируем перед сохранением
	if err := tm.validateConfig(config); err != nil {
		return fmt.Errorf("invalid config: %v", err)
	}

	// Diff-based save: проверяем изменения
	if tm.lastSavedConfig != nil && tm.configsEqual(config, tm.lastSavedConfig) {
		// Конфигурация не изменилась, не сохраняем
		return nil
	}

	// Обновляем метаданные
	config.LastModified = time.Now()

	// Сериализуем в TOML
	var buf bytes.Buffer
	encoder := toml.NewEncoder(&buf)
	encoder.Indent = "  "

	if err := encoder.Encode(config); err != nil {
		return fmt.Errorf("cannot serialize config: %v", err)
	}

	// Создаем backup старого файла
	if err := tm.createBackup(); err != nil {
		// Логируем ошибку, но не прерываем сохранение
		fmt.Printf("Warning: cannot create config backup: %v\n", err)
	}

	// Записываем файл
	if err := ioutil.WriteFile(tm.tomlPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("cannot write config file: %v", err)
	}

	// Обновляем lastSavedConfig
	tm.lastSavedConfig = tm.copyConfig(config)

	return nil
}

// saveWorker асинхронный worker для сохранения конфигурации
func (tm *TOMLConfigManager) saveWorker() {
	for config := range tm.saveQueue {
		tm.saveMutex.Lock()
		tm.saveInProgress = true
		tm.saveMutex.Unlock()

		tm.mutex.Lock()
		err := tm.saveTOMLUnsafe(config)
		tm.mutex.Unlock()

		if err != nil {
			fmt.Printf("Error saving config: %v\n", err)
		}

		tm.saveMutex.Lock()
		tm.saveInProgress = false
		tm.saveMutex.Unlock()
	}
}

// configsEqual сравнивает две конфигурации (для diff-based save)
func (tm *TOMLConfigManager) configsEqual(c1, c2 *Config) bool {
	if c1 == nil || c2 == nil {
		return c1 == c2
	}

	// Используем reflect для глубокого сравнения
	// Игнорируем поля LastModified и ConfigPath
	v1 := reflect.ValueOf(*c1)
	v2 := reflect.ValueOf(*c2)

	return tm.deepEqual(v1, v2, map[string]bool{
		"LastModified": true,
		"ConfigPath":   true,
	})
}

// deepEqual глубокое сравнение значений с исключением полей
func (tm *TOMLConfigManager) deepEqual(v1, v2 reflect.Value, exclude map[string]bool) bool {
	if v1.Type() != v2.Type() {
		return false
	}

	switch v1.Kind() {
	case reflect.Struct:
		for i := 0; i < v1.NumField(); i++ {
			field := v1.Type().Field(i)
			if exclude[field.Name] {
				continue
			}
			if !tm.deepEqual(v1.Field(i), v2.Field(i), exclude) {
				return false
			}
		}
		return true

	case reflect.Slice, reflect.Array:
		if v1.Len() != v2.Len() {
			return false
		}
		for i := 0; i < v1.Len(); i++ {
			if !tm.deepEqual(v1.Index(i), v2.Index(i), exclude) {
				return false
			}
		}
		return true

	case reflect.Map:
		if v1.Len() != v2.Len() {
			return false
		}
		for _, key := range v1.MapKeys() {
			if !tm.deepEqual(v1.MapIndex(key), v2.MapIndex(key), exclude) {
				return false
			}
		}
		return true

	case reflect.Ptr:
		if v1.IsNil() != v2.IsNil() {
			return false
		}
		if v1.IsNil() {
			return true
		}
		return tm.deepEqual(v1.Elem(), v2.Elem(), exclude)

	default:
		return reflect.DeepEqual(v1.Interface(), v2.Interface())
	}
}

// SetDebounceDuration устанавливает длительность debouncing
func (tm *TOMLConfigManager) SetDebounceDuration(duration time.Duration) {
	tm.debounceMutex.Lock()
	defer tm.debounceMutex.Unlock()
	tm.debounceDuration = duration
}

// IsSaveInProgress проверяет, выполняется ли сохранение
func (tm *TOMLConfigManager) IsSaveInProgress() bool {
	tm.saveMutex.Lock()
	defer tm.saveMutex.Unlock()
	return tm.saveInProgress
}

// Flush сохраняет все pending изменения немедленно
func (tm *TOMLConfigManager) Flush() error {
	// Останавливаем debounce timer
	tm.debounceMutex.Lock()
	if tm.debounceTimer != nil {
		tm.debounceTimer.Stop()
		tm.debounceTimer = nil
	}
	tm.debounceMutex.Unlock()

	// Сохраняем текущую конфигурацию
	return tm.SaveConfigSync()
}

// Cleanup освобождает ресурсы
func (tm *TOMLConfigManager) Cleanup() {
	// Останавливаем debounce timer
	tm.debounceMutex.Lock()
	if tm.debounceTimer != nil {
		tm.debounceTimer.Stop()
		tm.debounceTimer = nil
	}
	tm.debounceMutex.Unlock()

	// Закрываем очередь сохранения
	close(tm.saveQueue)

	// Вызываем базовый Cleanup
	tm.ConfigManager.Cleanup()
}
