package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// EditorCommand представляет команду редактора
type EditorCommand interface {
	Execute(editor *EditorWidget) error
	Undo(editor *EditorWidget) error
	GetDescription() string
}

// CommandHistory управляет историей команд для undo/redo
type CommandHistory struct {
	commands     []EditorCommand
	currentIndex int
	maxSize      int
}

// NewCommandHistory создает новую историю команд
func NewCommandHistory(maxSize int) *CommandHistory {
	return &CommandHistory{
		commands:     make([]EditorCommand, 0, maxSize),
		currentIndex: -1,
		maxSize:      maxSize,
	}
}

// Execute выполняет команду и добавляет в историю
func (h *CommandHistory) Execute(cmd EditorCommand, editor *EditorWidget) error {
	err := cmd.Execute(editor)
	if err != nil {
		return err
	}

	// Удаляем команды после текущего индекса (для redo)
	if h.currentIndex < len(h.commands)-1 {
		h.commands = h.commands[:h.currentIndex+1]
	}

	// Добавляем новую команду
	h.commands = append(h.commands, cmd)
	h.currentIndex++

	// Ограничиваем размер истории
	if len(h.commands) > h.maxSize {
		h.commands = h.commands[1:]
		h.currentIndex--
	}

	return nil
}

// Undo отменяет последнюю команду
func (h *CommandHistory) Undo(editor *EditorWidget) error {
	if h.currentIndex < 0 {
		return fmt.Errorf("nothing to undo")
	}

	cmd := h.commands[h.currentIndex]
	err := cmd.Undo(editor)
	if err != nil {
		return err
	}

	h.currentIndex--
	return nil
}

// Redo повторяет отмененную команду
func (h *CommandHistory) Redo(editor *EditorWidget) error {
	if h.currentIndex >= len(h.commands)-1 {
		return fmt.Errorf("nothing to redo")
	}

	h.currentIndex++
	cmd := h.commands[h.currentIndex]
	return cmd.Execute(editor)
}

// InsertTextCommand - команда вставки текста
type InsertTextCommand struct {
	position TextPosition
	text     string
	oldText  string
}

func (c *InsertTextCommand) Execute(editor *EditorWidget) error {
	// Сохраняем старый текст для undo
	c.oldText = editor.textContent

	lines := strings.Split(editor.textContent, "\n")
	if c.position.Row >= 0 && c.position.Row < len(lines) {
		line := lines[c.position.Row]
		if c.position.Col >= 0 && c.position.Col <= len(line) {
			newLine := line[:c.position.Col] + c.text + line[c.position.Col:]
			lines[c.position.Row] = newLine
			editor.textContent = strings.Join(lines, "\n")
			editor.updateDisplay()
		}
	}

	return nil
}

func (c *InsertTextCommand) Undo(editor *EditorWidget) error {
	editor.textContent = c.oldText
	editor.updateDisplay()
	return nil
}

func (c *InsertTextCommand) GetDescription() string {
	return fmt.Sprintf("Insert text at %d:%d", c.position.Row, c.position.Col)
}

// DeleteTextCommand - команда удаления текста
type DeleteTextCommand struct {
	startPos    TextPosition
	endPos      TextPosition
	deletedText string
	oldText     string
}

func (c *DeleteTextCommand) Execute(editor *EditorWidget) error {
	c.oldText = editor.textContent

	lines := strings.Split(editor.textContent, "\n")

	// Удаляем текст между позициями
	start := c.startPos
	end := c.endPos

	// Нормализуем порядок позиций
	if start.Row > end.Row || (start.Row == end.Row && start.Col > end.Col) {
		start, end = end, start
	}

	if start.Row == end.Row {
		// Удаление в одной строке
		if start.Row >= 0 && start.Row < len(lines) {
			line := lines[start.Row]
			if start.Col >= 0 && end.Col <= len(line) {
				c.deletedText = line[start.Col:end.Col]
				newLine := line[:start.Col] + line[end.Col:]
				lines[start.Row] = newLine
			}
		}
	} else {
		// Удаление нескольких строк
		if start.Row >= 0 && end.Row < len(lines) {
			startLine := lines[start.Row]
			endLine := lines[end.Row]

			if start.Col < 0 || start.Col > len(startLine) {
				start.Col = len(startLine)
			}
			if end.Col < 0 || end.Col > len(endLine) {
				end.Col = len(endLine)
			}

			// Сохраняем удаленный текст
			parts := []string{startLine[start.Col:]}
			if end.Row-start.Row > 1 {
				parts = append(parts, lines[start.Row+1:end.Row]...)
			}
			parts = append(parts, endLine[:end.Col])
			c.deletedText = strings.Join(parts, "\n")

			// Формируем новую строку на месте удаленного фрагмента
			newLine := startLine[:start.Col] + endLine[end.Col:]
			lines = append(lines[:start.Row], append([]string{newLine}, lines[end.Row+1:]...)...)
		}
	}

	editor.textContent = strings.Join(lines, "\n")
	editor.updateDisplay()
	return nil
}

func (c *DeleteTextCommand) Undo(editor *EditorWidget) error {
	editor.textContent = c.oldText
	editor.updateDisplay()
	return nil
}

func (c *DeleteTextCommand) GetDescription() string {
	return fmt.Sprintf("Delete text from %d:%d to %d:%d",
		c.startPos.Row, c.startPos.Col, c.endPos.Row, c.endPos.Col)
}

// ReplaceTextCommand - команда замены текста
type ReplaceTextCommand struct {
	findText    string
	replaceText string
	positions   []TextPosition
	oldText     string
}

func (c *ReplaceTextCommand) Execute(editor *EditorWidget) error {
	c.oldText = editor.textContent

	if c.findText == "" {
		return fmt.Errorf("find text is empty")
	}

	newContent := strings.ReplaceAll(editor.textContent, c.findText, c.replaceText)
	editor.textContent = newContent
	editor.updateDisplay()

	return nil
}

func (c *ReplaceTextCommand) Undo(editor *EditorWidget) error {
	editor.textContent = c.oldText
	editor.updateDisplay()
	return nil
}

func (c *ReplaceTextCommand) GetDescription() string {
	return fmt.Sprintf("Replace '%s' with '%s'", c.findText, c.replaceText)
}

// FormatCodeCommand - команда форматирования кода
type FormatCodeCommand struct {
	language string
	oldText  string
}

func (c *FormatCodeCommand) Execute(editor *EditorWidget) error {
	c.oldText = editor.textContent

	// Форматируем в зависимости от языка
	formattedText, err := formatCode(editor.textContent, c.language)
	if err != nil {
		return err
	}

	editor.textContent = formattedText
	editor.updateDisplay()
	return nil
}

func (c *FormatCodeCommand) Undo(editor *EditorWidget) error {
	editor.textContent = c.oldText
	editor.updateDisplay()
	return nil
}

func (c *FormatCodeCommand) GetDescription() string {
	return fmt.Sprintf("Format %s code", c.language)
}

// formatCode форматирует код в зависимости от языка
func formatCode(code, language string) (string, error) {
	switch strings.ToLower(language) {
	case "go":
		// Используем gofmt, читая код из stdin и получая форматированный код из stdout
		return runFormatter("gofmt", nil, code)
	case "python":
		// Пытаемся использовать black, если он недоступен - пробуем autopep8
		if formatted, err := runFormatter("black", []string{"--quiet", "-"}, code); err == nil {
			return formatted, nil
		} else if formatted, err := runFormatter("autopep8", []string{"-"}, code); err == nil {
			return formatted, nil
		} else {
			return "", fmt.Errorf("python formatter not found: %w", err)
		}
	case "rust":
		// Форматирование через rustfmt
		return runFormatter("rustfmt", []string{"--emit=stdout"}, code)
	case "c":
		// Форматирование через clang-format
		return runFormatter("clang-format", []string{"-style=LLVM"}, code)
	case "java":
		// Форматирование через google-java-format
		return runFormatter("google-java-format", []string{"-"}, code)
	default:
		// Базовое форматирование отступов
		return formatIndentation(code), nil
	}
}

// runFormatter выполняет внешний форматтер, передавая ему код через stdin и возвращая результат из stdout
func runFormatter(cmdName string, args []string, code string) (string, error) {
	path, err := exec.LookPath(cmdName)
	if err != nil {
		return "", fmt.Errorf("%s not found", cmdName)
	}

	cmd := exec.Command(path, args...)
	cmd.Stdin = strings.NewReader(code)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("%s failed: %s", cmdName, strings.TrimSpace(stderr.String()))
		}
		return "", fmt.Errorf("%s failed: %w", cmdName, err)
	}

	return stdout.String(), nil
}

// formatIndentation выполняет базовое форматирование отступов
func formatIndentation(code string) string {
	lines := strings.Split(code, "\n")
	indentLevel := 0
	formatted := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			formatted = append(formatted, "")
			continue
		}

		// Уменьшаем отступ для закрывающих скобок
		if strings.HasPrefix(trimmed, "}") || strings.HasPrefix(trimmed, "]") || strings.HasPrefix(trimmed, ")") {
			if indentLevel > 0 {
				indentLevel--
			}
		}

		// Добавляем строку с отступом
		indent := strings.Repeat("    ", indentLevel)
		formatted = append(formatted, indent+trimmed)

		// Увеличиваем отступ для открывающих скобок
		if strings.HasSuffix(trimmed, "{") || strings.HasSuffix(trimmed, "[") || strings.HasSuffix(trimmed, "(") {
			indentLevel++
		}
	}

	return strings.Join(formatted, "\n")
}

// CommentCommand - команда комментирования кода
type CommentCommand struct {
	startLine int
	endLine   int
	language  string
	oldText   string
}

func (c *CommentCommand) Execute(editor *EditorWidget) error {
	c.oldText = editor.textContent

	lines := strings.Split(editor.textContent, "\n")
	commentPrefix := getCommentPrefix(c.language)

	for i := c.startLine; i <= c.endLine && i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "" {
			if strings.HasPrefix(strings.TrimSpace(lines[i]), commentPrefix) {
				// Убираем комментарий
				lines[i] = strings.Replace(lines[i], commentPrefix+" ", "", 1)
				lines[i] = strings.Replace(lines[i], commentPrefix, "", 1)
			} else {
				// Добавляем комментарий
				lines[i] = commentPrefix + " " + lines[i]
			}
		}
	}

	editor.textContent = strings.Join(lines, "\n")
	editor.updateDisplay()
	return nil
}

func (c *CommentCommand) Undo(editor *EditorWidget) error {
	editor.textContent = c.oldText
	editor.updateDisplay()
	return nil
}

func (c *CommentCommand) GetDescription() string {
	return fmt.Sprintf("Toggle comment lines %d-%d", c.startLine, c.endLine)
}

// getCommentPrefix возвращает префикс комментария для языка
func getCommentPrefix(language string) string {
	switch language {
	case "go", "rust", "c", "java", "javascript", "typescript":
		return "//"
	case "python", "ruby", "perl":
		return "#"
	case "sql", "lua":
		return "--"
	default:
		return "//"
	}
}

// FindCommand - команда поиска
type FindCommand struct {
	searchTerm     string
	caseSensitive  bool
	wholeWord      bool
	useRegex       bool
	foundPositions []TextPosition
}

func (c *FindCommand) Execute(editor *EditorWidget) error {
	c.foundPositions = []TextPosition{}

	if c.searchTerm == "" {
		return fmt.Errorf("search term is empty")
	}

	content := editor.textContent
	searchTerm := c.searchTerm

	if !c.caseSensitive {
		content = strings.ToLower(content)
		searchTerm = strings.ToLower(searchTerm)
	}

	if c.useRegex {
		// Поиск с regex
		re, err := regexp.Compile(searchTerm)
		if err != nil {
			return fmt.Errorf("invalid regex: %v", err)
		}

		lines := strings.Split(content, "\n")
		for row, line := range lines {
			matches := re.FindAllStringIndex(line, -1)
			for _, match := range matches {
				c.foundPositions = append(c.foundPositions, TextPosition{
					Row: row,
					Col: match[0],
				})
			}
		}
	} else if c.wholeWord {
		// Поиск целых слов
		wordRegex := regexp.MustCompile(`\b` + regexp.QuoteMeta(searchTerm) + `\b`)
		lines := strings.Split(content, "\n")
		for row, line := range lines {
			matches := wordRegex.FindAllStringIndex(line, -1)
			for _, match := range matches {
				c.foundPositions = append(c.foundPositions, TextPosition{
					Row: row,
					Col: match[0],
				})
			}
		}
	} else {
		// Простой поиск подстроки
		lines := strings.Split(content, "\n")
		for row, line := range lines {
			col := 0
			for {
				index := strings.Index(line[col:], searchTerm)
				if index == -1 {
					break
				}
				c.foundPositions = append(c.foundPositions, TextPosition{
					Row: row,
					Col: col + index,
				})
				col += index + len(searchTerm)
			}
		}
	}

	// Обновляем результаты поиска в редакторе
	editor.searchResults = []TextRange{}
	for _, pos := range c.foundPositions {
		editor.searchResults = append(editor.searchResults, TextRange{
			Start: pos,
			End:   TextPosition{Row: pos.Row, Col: pos.Col + len(c.searchTerm)},
			Text:  c.searchTerm,
		})
	}

	editor.updateDisplay()
	return nil
}

func (c *FindCommand) Undo(editor *EditorWidget) error {
	editor.searchResults = []TextRange{}
	editor.updateDisplay()
	return nil
}

func (c *FindCommand) GetDescription() string {
	return fmt.Sprintf("Find '%s' (%d matches)", c.searchTerm, len(c.foundPositions))
}

// GoToLineCommand - команда перехода к строке
type GoToLineCommand struct {
	lineNumber  int
	oldLine     int
	oldPosition TextPosition
}

func (c *GoToLineCommand) Execute(editor *EditorWidget) error {
	c.oldPosition = TextPosition{
		Row: editor.cursorRow,
		Col: editor.cursorCol,
	}

	lines := strings.Split(editor.textContent, "\n")
	if c.lineNumber < 1 || c.lineNumber > len(lines) {
		return fmt.Errorf("line number %d out of range (1-%d)", c.lineNumber, len(lines))
	}

	editor.cursorRow = c.lineNumber - 1
	editor.cursorCol = 0

	// Прокручиваем контейнер, чтобы строка была видна
	if editor.scrollContainer != nil {
		lineHeight := theme.TextSize()
		editor.scrollContainer.ScrollToOffset(fyne.NewPos(0, float32(editor.cursorRow)*lineHeight))
	}
	editor.updateDisplay()

	return nil
}

func (c *GoToLineCommand) Undo(editor *EditorWidget) error {
	editor.cursorRow = c.oldPosition.Row
	editor.cursorCol = c.oldPosition.Col
	editor.updateDisplay()
	return nil
}

func (c *GoToLineCommand) GetDescription() string {
	return fmt.Sprintf("Go to line %d", c.lineNumber)
}

// SelectAllCommand - команда выделения всего текста
type SelectAllCommand struct {
	oldSelection TextRange
}

func (c *SelectAllCommand) Execute(editor *EditorWidget) error {
	c.oldSelection = TextRange{
		Start: editor.selectionStart,
		End:   editor.selectionEnd,
	}

	lines := strings.Split(editor.textContent, "\n")

	editor.selectionStart = TextPosition{Row: 0, Col: 0}
	editor.selectionEnd = TextPosition{
		Row: len(lines) - 1,
		Col: len(lines[len(lines)-1]),
	}

	editor.updateDisplay()
	return nil
}

func (c *SelectAllCommand) Undo(editor *EditorWidget) error {
	editor.selectionStart = c.oldSelection.Start
	editor.selectionEnd = c.oldSelection.End
	editor.updateDisplay()
	return nil
}

func (c *SelectAllCommand) GetDescription() string {
	return "Select all text"
}
