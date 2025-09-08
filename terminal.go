package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

// TerminalManager управляет терминалами
type TerminalManager struct {
	config          *Config
	terminals       []*TerminalInstance
	activeTerminal  *TerminalInstance
	mutex           sync.RWMutex
	defaultShell    string
	workingDir      string
	environmentVars map[string]string
	visible         bool
}

// TerminalInstance представляет экземпляр терминала
type TerminalInstance struct {
	ID           string
	Type         TerminalType
	Process      *exec.Cmd
	StdinPipe    io.WriteCloser
	StdoutPipe   io.ReadCloser
	StderrPipe   io.ReadCloser
	WorkingDir   string
	IsRunning    bool
	Window       fyne.Window
	Output       *widget.Entry
	OutputScroll *container.Scroll
	Input        *widget.Entry
	OutputBuffer strings.Builder
	mutex        sync.Mutex
}

// TerminalType тип терминала
type TerminalType int

const (
	TerminalCMD TerminalType = iota
	TerminalPowerShell
	TerminalBash
	TerminalCustom
	TerminalDefault
)

// NewTerminalManager создает новый менеджер терминалов
func NewTerminalManager(config *Config) *TerminalManager {
	tm := &TerminalManager{
		config:          config,
		terminals:       make([]*TerminalInstance, 0),
		environmentVars: make(map[string]string),
	}

	// Определяем оболочку по умолчанию
	tm.detectDefaultShell()

	// Устанавливаем рабочую директорию
	if config.ExternalTools.WorkingDirectory != "" {
		tm.workingDir = config.ExternalTools.WorkingDirectory
	} else {
		tm.workingDir, _ = os.Getwd()
	}

	return tm
}

// detectDefaultShell определяет оболочку по умолчанию
func (tm *TerminalManager) detectDefaultShell() {
	if runtime.GOOS == "windows" {
		// На Windows проверяем доступность PowerShell
		if _, err := exec.LookPath("powershell.exe"); err == nil {
			tm.defaultShell = "powershell"
		} else {
			tm.defaultShell = "cmd"
		}

		// Переопределяем из конфигурации если задано
		if tm.config != nil && tm.config.ExternalTools.DefaultTerminal != "" {
			tm.defaultShell = tm.config.ExternalTools.DefaultTerminal
		}
	} else {
		// На других ОС используем bash
		tm.defaultShell = "bash"
	}
}

// OpenTerminal открывает новый терминал
func (tm *TerminalManager) OpenTerminal(terminalType TerminalType, workingDir string) (*TerminalInstance, error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	if terminalType == TerminalDefault {
		switch strings.ToLower(tm.defaultShell) {
		case "cmd":
			terminalType = TerminalCMD
		case "powershell":
			terminalType = TerminalPowerShell
		case "bash":
			terminalType = TerminalBash
		default:
			terminalType = TerminalCustom
			if tm.config.ExternalTools.TerminalPath == "" {
				tm.config.ExternalTools.TerminalPath = tm.defaultShell
			}
		}
	}

	// Создаем новый экземпляр терминала
	terminal := &TerminalInstance{
		ID:         fmt.Sprintf("terminal_%d", time.Now().Unix()),
		Type:       terminalType,
		WorkingDir: workingDir,
		IsRunning:  false,
	}

	// Если не указана рабочая директория, используем текущую
	if terminal.WorkingDir == "" {
		terminal.WorkingDir = tm.workingDir
	}

	// Запускаем процесс терминала
	if err := tm.startTerminalProcess(terminal); err != nil {
		return nil, err
	}

	// Создаем окно для терминала
	tm.createTerminalWindow(terminal)

	// Добавляем в список
	tm.terminals = append(tm.terminals, terminal)
	tm.activeTerminal = terminal

	// Запускаем чтение вывода
	go tm.readTerminalOutput(terminal)
	go tm.readTerminalError(terminal)

	return terminal, nil
}

// startTerminalProcess запускает процесс терминала
func (tm *TerminalManager) startTerminalProcess(terminal *TerminalInstance) error {
	var cmd *exec.Cmd

	switch terminal.Type {
	case TerminalCMD:
		cmd = exec.Command("cmd.exe", "/K", "chcp 65001 >nul && prompt $P$G")
	case TerminalPowerShell:
		cmd = exec.Command("powershell.exe", "-NoExit", "-ExecutionPolicy", "Bypass", "-Command", "chcp 65001; [Console]::OutputEncoding = [System.Text.Encoding]::UTF8; $OutputEncoding = [Console]::OutputEncoding")
	case TerminalBash:
		if runtime.GOOS == "windows" {
			// Пытаемся найти Git Bash или WSL
			if gitBashPath := tm.findGitBash(); gitBashPath != "" {
				cmd = exec.Command(gitBashPath)
			} else if wslPath, err := exec.LookPath("wsl.exe"); err == nil {
				cmd = exec.Command(wslPath)
			} else {
				return fmt.Errorf("bash not found on Windows")
			}
		} else {
			cmd = exec.Command("bash")
		}
	case TerminalCustom:
		if tm.config.ExternalTools.TerminalPath != "" {
			args := strings.Fields(tm.config.ExternalTools.TerminalArgs)
			cmd = exec.Command(tm.config.ExternalTools.TerminalPath, args...)
		} else {
			return fmt.Errorf("custom terminal path not configured")
		}
	default:
		return fmt.Errorf("unknown terminal type")
	}

	// Устанавливаем рабочую директорию
	cmd.Dir = terminal.WorkingDir

	// Устанавливаем переменные окружения
	cmd.Env = os.Environ()
	for key, value := range tm.environmentVars {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
	}

	// Создаем пайпы для ввода/вывода
	var err error
	terminal.StdinPipe, err = cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %v", err)
	}

	terminal.StdoutPipe, err = cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %v", err)
	}

	terminal.StderrPipe, err = cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %v", err)
	}

	// Запускаем процесс
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start terminal process: %v", err)
	}

	terminal.Process = cmd
	terminal.IsRunning = true

	return nil
}

// createTerminalWindow создает окно для терминала
func (tm *TerminalManager) createTerminalWindow(terminal *TerminalInstance) {
	app := fyne.CurrentApp()

	// Создаем новое окно
	title := tm.getTerminalTitle(terminal)
	window := app.NewWindow(title)
	window.Resize(fyne.NewSize(800, 600))

	// Создаем виджеты вывода и ввода
	output := widget.NewMultiLineEntry()
	output.Disable() // Только для чтения
	output.SetText("")
	terminal.Output = output

	input := widget.NewEntry()
	input.SetPlaceHolder("Type command and press Enter...")
	terminal.Input = input

	// Обработчик ввода команд
	input.OnSubmitted = func(text string) {
		if text != "" {
			tm.sendCommand(terminal, text)
			input.SetText("")
		}
	}

	// Создаем toolbar
	toolbar := widget.NewToolbar(
		widget.NewToolbarAction(theme.ViewRefreshIcon(), func() {
			tm.clearTerminal(terminal)
		}),
		widget.NewToolbarSeparator(),
		widget.NewToolbarAction(theme.FolderOpenIcon(), func() {
			tm.changeDirectory(terminal)
		}),
		widget.NewToolbarAction(theme.ContentCopyIcon(), func() {
			tm.copyOutput(terminal)
		}),
		widget.NewToolbarSeparator(),
		widget.NewToolbarAction(theme.DeleteIcon(), func() {
			tm.closeTerminal(terminal)
			window.Close()
		}),
	)

	// Создаем layout
	outputScroll := container.NewScroll(output)
	outputScroll.SetMinSize(fyne.NewSize(800, 500))
	terminal.OutputScroll = outputScroll

	content := container.NewBorder(
		toolbar,
		input,
		nil,
		nil,
		outputScroll,
	)

	window.SetContent(content)
	terminal.Window = window

	// Обработчик закрытия окна
	window.SetOnClosed(func() {
		tm.closeTerminal(terminal)
	})

	window.Show()
	tm.visible = true

	// Фокус на поле ввода
	window.Canvas().Focus(input)
}

// getTerminalTitle возвращает заголовок окна терминала
func (tm *TerminalManager) getTerminalTitle(terminal *TerminalInstance) string {
	var typeStr string
	switch terminal.Type {
	case TerminalCMD:
		typeStr = "CMD"
	case TerminalPowerShell:
		typeStr = "PowerShell"
	case TerminalBash:
		typeStr = "Bash"
	case TerminalCustom:
		typeStr = "Terminal"
	}

	return fmt.Sprintf("%s - %s", typeStr, filepath.Base(terminal.WorkingDir))
}

// sendCommand отправляет команду в терминал
func (tm *TerminalManager) sendCommand(terminal *TerminalInstance, command string) error {
	if !terminal.IsRunning || terminal.StdinPipe == nil {
		return fmt.Errorf("terminal is not running")
	}

	// Добавляем команду в вывод для отображения
	terminal.mutex.Lock()
	terminal.OutputBuffer.WriteString(fmt.Sprintf("> %s\n", command))
	terminal.mutex.Unlock()
	tm.updateTerminalOutput(terminal)

	// Отправляем команду в терминал
	_, err := fmt.Fprintln(terminal.StdinPipe, command)
	return err
}

// readTerminalOutput читает stdout терминала
func (tm *TerminalManager) readTerminalOutput(terminal *TerminalInstance) {
	var reader io.Reader = terminal.StdoutPipe
	if runtime.GOOS == "windows" {
		// После смены кодовой страницы на UTF-8 (65001) для CMD и PowerShell
		// декодирование из CP866 приводит к искажению выводимых символов.
		// Поэтому применяем преобразование только для терминалов, где кодировка
		// остается CP866 (например, сторонние оболочки).
		if terminal.Type != TerminalCMD && terminal.Type != TerminalPowerShell {
			reader = transform.NewReader(terminal.StdoutPipe, charmap.CodePage866.NewDecoder())
		}
	}
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		if !terminal.IsRunning {
			break
		}

		line := scanner.Text()
		terminal.mutex.Lock()
		terminal.OutputBuffer.WriteString(line + "\n")
		terminal.mutex.Unlock()

		tm.updateTerminalOutput(terminal)
	}
}

// readTerminalError читает stderr терминала
func (tm *TerminalManager) readTerminalError(terminal *TerminalInstance) {
	var reader io.Reader = terminal.StderrPipe
	if runtime.GOOS == "windows" {
		// Аналогично stdout, преобразуем только если терминал не переключен на UTF-8
		if terminal.Type != TerminalCMD && terminal.Type != TerminalPowerShell {
			reader = transform.NewReader(terminal.StderrPipe, charmap.CodePage866.NewDecoder())
		}
	}
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		if !terminal.IsRunning {
			break
		}

		line := scanner.Text()
		terminal.mutex.Lock()
		terminal.OutputBuffer.WriteString(fmt.Sprintf("[ERROR] %s\n", line))
		terminal.mutex.Unlock()

		tm.updateTerminalOutput(terminal)
	}
}

// updateTerminalOutput обновляет отображение вывода терминала
func (tm *TerminalManager) updateTerminalOutput(terminal *TerminalInstance) {
	if terminal.Output == nil {
		return
	}

	terminal.mutex.Lock()
	content := terminal.OutputBuffer.String()
	terminal.mutex.Unlock()

	// Обновляем вывод в UI-потоке
	fyne.Do(func() {
		terminal.Output.SetText(content)

		// Прокручиваем к концу
		terminal.Output.CursorRow = len(strings.Split(content, "\n")) - 1
		if terminal.OutputScroll != nil {
			terminal.OutputScroll.ScrollToBottom()
		}
	})
}

// clearTerminal очищает вывод терминала
func (tm *TerminalManager) clearTerminal(terminal *TerminalInstance) {
	terminal.mutex.Lock()
	terminal.OutputBuffer.Reset()
	terminal.mutex.Unlock()

	if terminal.Output != nil {
		terminal.Output.SetText("")
	}

	// Отправляем команду очистки в зависимости от типа терминала
	switch terminal.Type {
	case TerminalCMD:
		tm.sendCommand(terminal, "cls")
	case TerminalPowerShell:
		tm.sendCommand(terminal, "Clear-Host")
	case TerminalBash:
		tm.sendCommand(terminal, "clear")
	}
}

// changeDirectory открывает диалог смены директории
func (tm *TerminalManager) changeDirectory(terminal *TerminalInstance) {
	if terminal == nil || terminal.Window == nil {
		return
	}

	folderDialog := dialog.NewFolderOpen(func(list fyne.ListableURI, err error) {
		if err != nil {
			dialog.ShowError(err, terminal.Window)
			return
		}
		if list == nil {
			return
		}

		path := list.Path()

		var cmd string
		switch terminal.Type {
		case TerminalCMD:
			cmd = fmt.Sprintf("cd /d \"%s\"", path)
		case TerminalPowerShell:
			cmd = fmt.Sprintf("Set-Location -Path '%s'", path)
		default: // TerminalBash and others
			cmd = fmt.Sprintf("cd \"%s\"", path)
		}

		if err := tm.sendCommand(terminal, cmd); err != nil {
			dialog.ShowError(err, terminal.Window)
			return
		}

		terminal.WorkingDir = path
		tm.SetWorkingDirectory(path)

		if terminal.Window != nil {
			terminal.Window.SetTitle(tm.getTerminalTitle(terminal))
		}
	}, terminal.Window)

	if terminal.WorkingDir != "" {
		if uri, err := storage.ListerForURI(storage.NewFileURI(terminal.WorkingDir)); err == nil {
			folderDialog.SetLocation(uri)
		}
	}

	folderDialog.Show()
}

// copyOutput копирует вывод терминала в буфер обмена
func (tm *TerminalManager) copyOutput(terminal *TerminalInstance) {
	if terminal.Output == nil || terminal.Window == nil {
		return
	}

	terminal.mutex.Lock()
	content := terminal.OutputBuffer.String()
	terminal.mutex.Unlock()

	if content != "" {
		clipboard := terminal.Window.Clipboard()
		clipboard.SetContent(content)
	}
}

// closeTerminal закрывает терминал
func (tm *TerminalManager) closeTerminal(terminal *TerminalInstance) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	// Останавливаем процесс
	if terminal.IsRunning && terminal.Process != nil {
		terminal.IsRunning = false

		// Закрываем пайпы
		if terminal.StdinPipe != nil {
			terminal.StdinPipe.Close()
		}
		if terminal.StdoutPipe != nil {
			terminal.StdoutPipe.Close()
		}
		if terminal.StderrPipe != nil {
			terminal.StderrPipe.Close()
		}

		// Завершаем процесс
		terminal.Process.Process.Kill()
		terminal.Process.Wait()
	}

	// Удаляем из списка
	for i, t := range tm.terminals {
		if t.ID == terminal.ID {
			tm.terminals = append(tm.terminals[:i], tm.terminals[i+1:]...)
			break
		}
	}

	// Обновляем активный терминал
	if tm.activeTerminal == terminal {
		if len(tm.terminals) > 0 {
			tm.activeTerminal = tm.terminals[len(tm.terminals)-1]
		} else {
			tm.activeTerminal = nil
			tm.visible = false
		}
	}
}

// CloseAllTerminals закрывает все терминалы
func (tm *TerminalManager) CloseAllTerminals() {
	tm.mutex.Lock()
	terminals := make([]*TerminalInstance, len(tm.terminals))
	copy(terminals, tm.terminals)
	tm.mutex.Unlock()

	for _, terminal := range terminals {
		if terminal.Window != nil {
			terminal.Window.Close()
		}
		tm.closeTerminal(terminal)
	}
	tm.visible = false
}

// findGitBash ищет Git Bash на Windows
func (tm *TerminalManager) findGitBash() string {
	possiblePaths := []string{
		`C:\Program Files\Git\bin\bash.exe`,
		`C:\Program Files (x86)\Git\bin\bash.exe`,
		`C:\Git\bin\bash.exe`,
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Пытаемся найти через переменную окружения
	if gitPath := os.Getenv("GIT_INSTALL_ROOT"); gitPath != "" {
		bashPath := filepath.Join(gitPath, "bin", "bash.exe")
		if _, err := os.Stat(bashPath); err == nil {
			return bashPath
		}
	}

	return ""
}

// SetWorkingDirectory устанавливает рабочую директорию
func (tm *TerminalManager) SetWorkingDirectory(dir string) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	tm.workingDir = dir
}

// SetEnvironmentVariable устанавливает переменную окружения
func (tm *TerminalManager) SetEnvironmentVariable(key, value string) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	tm.environmentVars[key] = value
}

// GetActiveTerminal возвращает активный терминал
func (tm *TerminalManager) GetActiveTerminal() *TerminalInstance {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()

	return tm.activeTerminal
}

// GetTerminals возвращает список всех терминалов
func (tm *TerminalManager) GetTerminals() []*TerminalInstance {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()

	result := make([]*TerminalInstance, len(tm.terminals))
	copy(result, tm.terminals)
	return result
}

// RunCommand выполняет команду в новом терминале и возвращает результат
func (tm *TerminalManager) RunCommand(command string, workingDir string) (string, error) {
	// Определяем оболочку
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}

	if workingDir != "" {
		cmd.Dir = workingDir
	} else {
		cmd.Dir = tm.workingDir
	}

	// Выполняем команду
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// OpenCMD открывает CMD терминал
func (tm *TerminalManager) OpenCMD(workingDir string) error {
	_, err := tm.OpenTerminal(TerminalCMD, workingDir)
	return err
}

// OpenPowerShell открывает PowerShell терминал
func (tm *TerminalManager) OpenPowerShell(workingDir string) error {
	_, err := tm.OpenTerminal(TerminalPowerShell, workingDir)
	return err
}

// OpenBash открывает Bash терминал
func (tm *TerminalManager) OpenBash(workingDir string) error {
	_, err := tm.OpenTerminal(TerminalBash, workingDir)
	return err
}

// OpenCustomTerminal открывает пользовательский терминал
func (tm *TerminalManager) OpenCustomTerminal(workingDir string) error {
	_, err := tm.OpenTerminal(TerminalCustom, workingDir)
	return err
}

// IsVisible возвращает состояние видимости терминала
func (tm *TerminalManager) IsVisible() bool {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()
	return tm.visible
}

// Show отображает активный терминал, если он скрыт
func (tm *TerminalManager) Show() {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()
	if tm.activeTerminal != nil && tm.activeTerminal.Window != nil {
		tm.activeTerminal.Window.Show()
		tm.visible = true
	}
}

// Hide скрывает активный терминал
func (tm *TerminalManager) Hide() {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()
	if tm.activeTerminal != nil && tm.activeTerminal.Window != nil {
		tm.activeTerminal.Window.Hide()
		tm.visible = false
	}
}
