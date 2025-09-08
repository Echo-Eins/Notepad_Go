package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// Snippet представляет текстовый шаблон, который можно расширить по триггеру.
type Snippet struct {
	Trigger string `json:"trigger"`
	Body    string `json:"body"`
}

// SnippetManager управляет коллекцией сниппетов и их расширением.
type SnippetManager struct {
	snippets map[string]Snippet
	mutex    sync.RWMutex
}

// NewSnippetManager создает новый менеджер сниппетов.
func NewSnippetManager() *SnippetManager {
	return &SnippetManager{snippets: make(map[string]Snippet)}
}

// Add добавляет сниппет в менеджер.
func (sm *SnippetManager) Add(snippet Snippet) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	sm.snippets[snippet.Trigger] = snippet
}

// LoadFromFile загружает сниппеты из JSON файла.
func (sm *SnippetManager) LoadFromFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return sm.Load(f)
}

// Load загружает сниппеты из потока.
func (sm *SnippetManager) Load(r io.Reader) error {
	var list []Snippet
	if err := json.NewDecoder(r).Decode(&list); err != nil {
		return err
	}
	for _, s := range list {
		sm.Add(s)
	}
	return nil
}

// Expand возвращает тело сниппета по триггеру с подстановкой переменных.
func (sm *SnippetManager) Expand(trigger string, vars map[string]string) (string, bool) {
	sm.mutex.RLock()
	snippet, ok := sm.snippets[trigger]
	sm.mutex.RUnlock()
	if !ok {
		return "", false
	}
	result := snippet.Body
	for k, v := range vars {
		placeholder := fmt.Sprintf("${%s}", k)
		result = strings.ReplaceAll(result, placeholder, v)
	}
	return result, true
}
