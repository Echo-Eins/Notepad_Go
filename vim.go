package main

import (
	"fmt"
	"strings"
)

// VimHandler управляет Vim режимом в редакторе
type VimHandler struct {
	editor         *EditorWidget
	mode           VimMode
	pendingCommand string
	count          int
	register       string
	yankBuffer     string
	visualStart    TextPosition
	visualEnd      TextPosition
	searchPattern  string
	searchBackward bool
	marks          map[string]TextPosition
	jumpList       []TextPosition
	jumpIndex      int
	lastCommand    string
	recording      bool
	macroRegister  string
	macroCommands  []string
	macros         map[string][]string
}

// NewVimHandler создает новый обработчик Vim
func NewVimHandler(editor *EditorWidget) *VimHandler {
	return &VimHandler{
		editor:   editor,
		mode:     VimNormal,
		marks:    make(map[string]TextPosition),
		jumpList: make([]TextPosition, 0),
		macros:   make(map[string][]string),
		register: "\"",
	}
}

// HandleKey обрабатывает нажатие клавиши в Vim режиме
func (vh *VimHandler) HandleKey(key string) bool {
	// Если записываем макрос, добавляем команду
	if vh.recording && key != "q" {
		vh.macroCommands = append(vh.macroCommands, key)
	}

	switch vh.mode {
	case VimNormal:
		return vh.handleNormalMode(key)
	case VimInsert:
		return vh.handleInsertMode(key)
	case VimVisual:
		return vh.handleVisualMode(key)
	case VimCommand:
		return vh.handleCommandMode(key)
	case VimReplace:
		return vh.handleReplaceMode(key)
	default:
		return false
	}
}

// handleNormalMode обрабатывает Normal режим
func (vh *VimHandler) handleNormalMode(key string) bool {
	// ESC сбрасывает pending команды
	if key == "Escape" {
		vh.pendingCommand = ""
		vh.count = 0
		return true
	}

	// Числовые префиксы
	if key >= "0" && key <= "9" {
		if vh.pendingCommand == "" && key == "0" {
			// 0 - переход к началу строки
			vh.moveToLineStart()
		} else {
			// Накапливаем счетчик
			vh.count = vh.count*10 + int(key[0]-'0')
		}
		return true
	}

	// Добавляем к pending команде
	vh.pendingCommand += key

	// Обрабатываем команды
	handled := vh.executeNormalCommand(vh.pendingCommand)

	if handled {
		vh.lastCommand = vh.pendingCommand
		vh.pendingCommand = ""
		vh.count = 0
	}

	return handled
}

// executeNormalCommand выполняет команду в Normal режиме
func (vh *VimHandler) executeNormalCommand(cmd string) bool {
	count := vh.count
	if count == 0 {
		count = 1
	}

	switch cmd {
	// Движение
	case "h":
		for i := 0; i < count; i++ {
			vh.moveLeft()
		}
		return true
	case "j":
		for i := 0; i < count; i++ {
			vh.moveDown()
		}
		return true
	case "k":
		for i := 0; i < count; i++ {
			vh.moveUp()
		}
		return true
	case "l":
		for i := 0; i < count; i++ {
			vh.moveRight()
		}
		return true

	// Движение по словам
	case "w":
		for i := 0; i < count; i++ {
			vh.moveWordForward()
		}
		return true
	case "b":
		for i := 0; i < count; i++ {
			vh.moveWordBackward()
		}
		return true
	case "e":
		for i := 0; i < count; i++ {
			vh.moveWordEnd()
		}
		return true

	// Движение по строке
	case "0":
		vh.moveToLineStart()
		return true
	case "$":
		vh.moveToLineEnd()
		return true
	case "^":
		vh.moveToLineFirstNonBlank()
		return true

	// Переход к строке
	case "G":
		if vh.count > 0 {
			vh.goToLine(vh.count)
		} else {
			vh.goToLastLine()
		}
		return true
	case "gg":
		if vh.count > 0 {
			vh.goToLine(vh.count)
		} else {
			vh.goToLine(1)
		}
		return true

	// Режимы
	case "i":
		vh.mode = VimInsert
		return true
	case "I":
		vh.moveToLineFirstNonBlank()
		vh.mode = VimInsert
		return true
	case "a":
		vh.moveRight()
		vh.mode = VimInsert
		return true
	case "A":
		vh.moveToLineEnd()
		vh.mode = VimInsert
		return true
	case "o":
		vh.insertLineBelow()
		vh.mode = VimInsert
		return true
	case "O":
		vh.insertLineAbove()
		vh.mode = VimInsert
		return true
	case "v":
		vh.startVisualMode()
		return true
	case "V":
		vh.startVisualLineMode()
		return true
	case "R":
		vh.mode = VimReplace
		return true

	// Удаление
	case "x":
		for i := 0; i < count; i++ {
			vh.deleteChar()
		}
		return true
	case "X":
		for i := 0; i < count; i++ {
			vh.deleteCharBefore()
		}
		return true
	case "dd":
		for i := 0; i < count; i++ {
			vh.deleteLine()
		}
		return true
	case "D":
		vh.deleteToLineEnd()
		return true
	case "dw":
		for i := 0; i < count; i++ {
			vh.deleteWord()
		}
		return true

	// Изменение
	case "cc":
		vh.changeLine()
		return true
	case "C":
		vh.changeToLineEnd()
		return true
	case "cw":
		vh.changeWord()
		return true
	case "r":
		// Ждем следующий символ для замены
		return false

	// Копирование
	case "yy":
		for i := 0; i < count; i++ {
			vh.yankLine()
		}
		return true
	case "Y":
		vh.yankLine()
		return true
	case "yw":
		vh.yankWord()
		return true

	// Вставка
	case "p":
		for i := 0; i < count; i++ {
			vh.pasteAfter()
		}
		return true
	case "P":
		for i := 0; i < count; i++ {
			vh.pasteBefore()
		}
		return true

	// Отмена/повтор
	case "u":
		vh.undo()
		return true
	case "Ctrl+r":
		vh.redo()
		return true
	case ".":
		vh.repeatLastCommand()
		return true

	// Поиск
	case "/":
		vh.startSearch(false)
		return true
	case "?":
		vh.startSearch(true)
		return true
	case "n":
		vh.searchNext()
		return true
	case "N":
		vh.searchPrevious()
		return true
	case "*":
		vh.searchWordUnderCursor(false)
		return true
	case "#":
		vh.searchWordUnderCursor(true)
		return true

	// Метки
	case "m":
		// Ждем следующий символ для метки
		return false

	// Макросы
	case "q":
		if vh.recording {
			vh.stopRecordingMacro()
		} else {
			// Ждем регистр для записи
			return false
		}
		return true
	case "@":
		// Ждем регистр для воспроизведения
		return false

	// Ex команды
	case ":":
		vh.mode = VimCommand
		vh.pendingCommand = ""
		return true

	// Прокрутка
	case "Ctrl+d":
		vh.scrollHalfPageDown()
		return true
	case "Ctrl+u":
		vh.scrollHalfPageUp()
		return true
	case "Ctrl+f":
		vh.scrollPageDown()
		return true
	case "Ctrl+b":
		vh.scrollPageUp()
		return true

	// Скобки
	case "%":
		vh.jumpToMatchingBracket()
		return true
	}

	// Проверяем составные команды
	if len(cmd) > 1 {
		// Команды с регистром
		if cmd[0] == '"' && len(cmd) == 2 {
			vh.register = string(cmd[1])
			vh.pendingCommand = ""
			return false // Ждем следующую команду
		}

		// Команды с движением (d, c, y)
		if cmd[0] == 'd' || cmd[0] == 'c' || cmd[0] == 'y' {
			// Пока не полная команда
			if len(cmd) == 1 {
				return false
			}
		}

		// Замена символа
		if len(cmd) == 2 && cmd[0] == 'r' {
			vh.replaceChar(rune(cmd[1]))
			return true
		}

		// Метки
		if len(cmd) == 2 && cmd[0] == 'm' {
			vh.setMark(string(cmd[1]))
			return true
		}
		if len(cmd) == 2 && cmd[0] == '\'' {
			vh.jumpToMark(string(cmd[1]))
			return true
		}

		// Макросы
		if len(cmd) == 2 && cmd[0] == 'q' {
			vh.startRecordingMacro(string(cmd[1]))
			return true
		}
		if len(cmd) == 2 && cmd[0] == '@' {
			vh.playMacro(string(cmd[1]))
			return true
		}
	}

	return false
}

// handleInsertMode обрабатывает Insert режим
func (vh *VimHandler) handleInsertMode(key string) bool {
	if key == "Escape" {
		vh.mode = VimNormal
		vh.moveLeft() // Vim перемещает курсор влево при выходе из Insert
		return true
	}

	// В Insert режиме передаем ввод редактору
	return false
}

// handleVisualMode обрабатывает Visual режим
func (vh *VimHandler) handleVisualMode(key string) bool {
	if key == "Escape" {
		vh.mode = VimNormal
		vh.clearSelection()
		return true
	}

	// Движение расширяет выделение
	switch key {
	case "h":
		vh.moveLeft()
		vh.updateSelection()
		return true
	case "j":
		vh.moveDown()
		vh.updateSelection()
		return true
	case "k":
		vh.moveUp()
		vh.updateSelection()
		return true
	case "l":
		vh.moveRight()
		vh.updateSelection()
		return true
	case "w":
		vh.moveWordForward()
		vh.updateSelection()
		return true
	case "b":
		vh.moveWordBackward()
		vh.updateSelection()
		return true

	// Операции с выделением
	case "d", "x":
		vh.deleteSelection()
		vh.mode = VimNormal
		return true
	case "y":
		vh.yankSelection()
		vh.mode = VimNormal
		return true
	case "c":
		vh.changeSelection()
		vh.mode = VimInsert
		return true
	}

	return false
}

// handleCommandMode обрабатывает Command режим
func (vh *VimHandler) handleCommandMode(key string) bool {
	if key == "Escape" {
		vh.mode = VimNormal
		vh.pendingCommand = ""
		return true
	}

	if key == "Return" || key == "Enter" {
		vh.executeExCommand(vh.pendingCommand)
		vh.mode = VimNormal
		vh.pendingCommand = ""
		return true
	}

	if key == "Backspace" {
		if len(vh.pendingCommand) > 0 {
			vh.pendingCommand = vh.pendingCommand[:len(vh.pendingCommand)-1]
		}
		return true
	}

	// Добавляем символ к команде
	vh.pendingCommand += key
	return true
}

// handleReplaceMode обрабатывает Replace режим
func (vh *VimHandler) handleReplaceMode(key string) bool {
	if key == "Escape" {
		vh.mode = VimNormal
		return true
	}

	// Заменяем символ под курсором
	vh.replaceChar(rune(key[0]))
	vh.moveRight()
	return true
}

// executeExCommand выполняет Ex команду
func (vh *VimHandler) executeExCommand(cmd string) {
	cmd = strings.TrimSpace(cmd)

	// Базовые Ex команды
	switch {
	case cmd == "w" || cmd == "write":
		vh.saveFile()
	case cmd == "q" || cmd == "quit":
		vh.quit()
	case cmd == "wq" || cmd == "x":
		vh.saveFile()
		vh.quit()
	case cmd == "q!":
		vh.forceQuit()
	case strings.HasPrefix(cmd, "e ") || strings.HasPrefix(cmd, "edit "):
		filename := strings.TrimSpace(cmd[2:])
		vh.openFile(filename)
	case strings.HasPrefix(cmd, "set "):
		vh.setOption(cmd[4:])
	case strings.HasPrefix(cmd, "s/"):
		vh.substitute(cmd)
	case strings.HasPrefix(cmd, "%s/"):
		vh.substituteAll(cmd[1:])
	default:
		// Проверяем, является ли это номером строки
		if lineNum := parseLineNumber(cmd); lineNum > 0 {
			vh.goToLine(lineNum)
		}
	}
}

// Методы движения курсора

func (vh *VimHandler) moveLeft() {
	if vh.editor.cursorCol > 0 {
		vh.editor.cursorCol--
	}
}

func (vh *VimHandler) moveRight() {
	lines := strings.Split(vh.editor.textContent, "\n")
	if vh.editor.cursorRow < len(lines) {
		line := lines[vh.editor.cursorRow]
		if vh.editor.cursorCol < len(line)-1 {
			vh.editor.cursorCol++
		}
	}
}

func (vh *VimHandler) moveUp() {
	if vh.editor.cursorRow > 0 {
		vh.editor.cursorRow--
		vh.adjustCursorColumn()
	}
}

func (vh *VimHandler) moveDown() {
	lines := strings.Split(vh.editor.textContent, "\n")
	if vh.editor.cursorRow < len(lines)-1 {
		vh.editor.cursorRow++
		vh.adjustCursorColumn()
	}
}

func (vh *VimHandler) adjustCursorColumn() {
	lines := strings.Split(vh.editor.textContent, "\n")
	if vh.editor.cursorRow < len(lines) {
		line := lines[vh.editor.cursorRow]
		if vh.editor.cursorCol >= len(line) {
			vh.editor.cursorCol = len(line) - 1
			if vh.editor.cursorCol < 0 {
				vh.editor.cursorCol = 0
			}
		}
	}
}

func (vh *VimHandler) moveWordForward() {
	lines := strings.Split(vh.editor.textContent, "\n")
	if vh.editor.cursorRow >= len(lines) {
		return
	}

	line := lines[vh.editor.cursorRow]
	col := vh.editor.cursorCol

	// Пропускаем текущее слово
	for col < len(line) && !isWordBoundary(rune(line[col])) {
		col++
	}

	// Пропускаем пробелы
	for col < len(line) && isWordBoundary(rune(line[col])) {
		col++
	}

	if col < len(line) {
		vh.editor.cursorCol = col
	} else if vh.editor.cursorRow < len(lines)-1 {
		// Переходим на следующую строку
		vh.editor.cursorRow++
		vh.editor.cursorCol = 0
	}
}

func (vh *VimHandler) moveWordBackward() {
	if vh.editor.cursorCol > 0 {
		lines := strings.Split(vh.editor.textContent, "\n")
		line := lines[vh.editor.cursorRow]
		col := vh.editor.cursorCol - 1

		// Пропускаем пробелы
		for col > 0 && isWordBoundary(rune(line[col])) {
			col--
		}

		// Идем до начала слова
		for col > 0 && !isWordBoundary(rune(line[col-1])) {
			col--
		}

		vh.editor.cursorCol = col
	} else if vh.editor.cursorRow > 0 {
		// Переходим на предыдущую строку
		vh.editor.cursorRow--
		lines := strings.Split(vh.editor.textContent, "\n")
		vh.editor.cursorCol = len(lines[vh.editor.cursorRow]) - 1
	}
}

func (vh *VimHandler) moveWordEnd() {
	lines := strings.Split(vh.editor.textContent, "\n")
	if vh.editor.cursorRow >= len(lines) {
		return
	}

	line := lines[vh.editor.cursorRow]
	col := vh.editor.cursorCol + 1

	// Пропускаем пробелы
	for col < len(line) && isWordBoundary(rune(line[col])) {
		col++
	}

	// Идем до конца слова
	for col < len(line) && !isWordBoundary(rune(line[col])) {
		col++
	}

	if col > 0 && col <= len(line) {
		vh.editor.cursorCol = col - 1
	}
}

func (vh *VimHandler) moveToLineStart() {
	vh.editor.cursorCol = 0
}

func (vh *VimHandler) moveToLineEnd() {
	lines := strings.Split(vh.editor.textContent, "\n")
	if vh.editor.cursorRow < len(lines) {
		line := lines[vh.editor.cursorRow]
		vh.editor.cursorCol = len(line) - 1
		if vh.editor.cursorCol < 0 {
			vh.editor.cursorCol = 0
		}
	}
}

func (vh *VimHandler) moveToLineFirstNonBlank() {
	lines := strings.Split(vh.editor.textContent, "\n")
	if vh.editor.cursorRow < len(lines) {
		line := lines[vh.editor.cursorRow]
		for i, ch := range line {
			if ch != ' ' && ch != '\t' {
				vh.editor.cursorCol = i
				return
			}
		}
		vh.editor.cursorCol = 0
	}
}

func (vh *VimHandler) goToLine(lineNum int) {
	lines := strings.Split(vh.editor.textContent, "\n")
	if lineNum > 0 && lineNum <= len(lines) {
		vh.addToJumpList()
		vh.editor.cursorRow = lineNum - 1
		vh.editor.cursorCol = 0
	}
}

func (vh *VimHandler) goToLastLine() {
	lines := strings.Split(vh.editor.textContent, "\n")
	vh.addToJumpList()
	vh.editor.cursorRow = len(lines) - 1
	vh.editor.cursorCol = 0
}

// Методы редактирования

func (vh *VimHandler) deleteChar() {
	lines := strings.Split(vh.editor.textContent, "\n")
	if vh.editor.cursorRow < len(lines) {
		line := lines[vh.editor.cursorRow]
		if vh.editor.cursorCol < len(line) {
			vh.yankBuffer = string(line[vh.editor.cursorCol])
			lines[vh.editor.cursorRow] = line[:vh.editor.cursorCol] + line[vh.editor.cursorCol+1:]
			vh.editor.textContent = strings.Join(lines, "\n")
			vh.editor.isDirty = true
			vh.editor.updateDisplay()
		}
	}
}

func (vh *VimHandler) deleteCharBefore() {
	if vh.editor.cursorCol > 0 {
		vh.moveLeft()
		vh.deleteChar()
	}
}

func (vh *VimHandler) deleteLine() {
	lines := strings.Split(vh.editor.textContent, "\n")
	if vh.editor.cursorRow < len(lines) {
		vh.yankBuffer = lines[vh.editor.cursorRow] + "\n"
		lines = append(lines[:vh.editor.cursorRow], lines[vh.editor.cursorRow+1:]...)
		vh.editor.textContent = strings.Join(lines, "\n")
		vh.editor.isDirty = true

		// Корректируем позицию курсора
		if vh.editor.cursorRow >= len(lines) && len(lines) > 0 {
			vh.editor.cursorRow = len(lines) - 1
		}
		vh.editor.cursorCol = 0
		vh.editor.updateDisplay()
	}
}

func (vh *VimHandler) deleteToLineEnd() {
	lines := strings.Split(vh.editor.textContent, "\n")
	if vh.editor.cursorRow < len(lines) {
		line := lines[vh.editor.cursorRow]
		if vh.editor.cursorCol < len(line) {
			vh.yankBuffer = line[vh.editor.cursorCol:]
			lines[vh.editor.cursorRow] = line[:vh.editor.cursorCol]
			vh.editor.textContent = strings.Join(lines, "\n")
			vh.editor.isDirty = true
			vh.editor.updateDisplay()
		}
	}
}

func (vh *VimHandler) deleteWord() {
	startCol := vh.editor.cursorCol
	vh.moveWordForward()
	endCol := vh.editor.cursorCol

	lines := strings.Split(vh.editor.textContent, "\n")
	if vh.editor.cursorRow < len(lines) {
		line := lines[vh.editor.cursorRow]
		if startCol < len(line) && endCol <= len(line) {
			vh.yankBuffer = line[startCol:endCol]
			lines[vh.editor.cursorRow] = line[:startCol] + line[endCol:]
			vh.editor.cursorCol = startCol
			vh.editor.textContent = strings.Join(lines, "\n")
			vh.editor.isDirty = true
			vh.editor.updateDisplay()
		}
	}
}

func (vh *VimHandler) yankLine() {
	lines := strings.Split(vh.editor.textContent, "\n")
	if vh.editor.cursorRow < len(lines) {
		vh.yankBuffer = lines[vh.editor.cursorRow] + "\n"
	}
}

func (vh *VimHandler) yankWord() {
	startCol := vh.editor.cursorCol
	vh.moveWordForward()
	endCol := vh.editor.cursorCol

	lines := strings.Split(vh.editor.textContent, "\n")
	if vh.editor.cursorRow < len(lines) {
		line := lines[vh.editor.cursorRow]
		if startCol < len(line) && endCol <= len(line) {
			vh.yankBuffer = line[startCol:endCol]
			vh.editor.cursorCol = startCol
		}
	}
}

func (vh *VimHandler) pasteAfter() {
	if vh.yankBuffer == "" {
		return
	}

	lines := strings.Split(vh.editor.textContent, "\n")

	if strings.HasSuffix(vh.yankBuffer, "\n") {
		// Вставка строки
		newLines := make([]string, 0, len(lines)+1)
		newLines = append(newLines, lines[:vh.editor.cursorRow+1]...)
		newLines = append(newLines, strings.TrimSuffix(vh.yankBuffer, "\n"))
		if vh.editor.cursorRow+1 < len(lines) {
			newLines = append(newLines, lines[vh.editor.cursorRow+1:]...)
		}
		vh.editor.textContent = strings.Join(newLines, "\n")
		vh.editor.cursorRow++
		vh.editor.cursorCol = 0
	} else {
		// Вставка текста
		if vh.editor.cursorRow < len(lines) {
			line := lines[vh.editor.cursorRow]
			insertPos := vh.editor.cursorCol + 1
			if insertPos > len(line) {
				insertPos = len(line)
			}
			lines[vh.editor.cursorRow] = line[:insertPos] + vh.yankBuffer + line[insertPos:]
			vh.editor.textContent = strings.Join(lines, "\n")
			vh.editor.cursorCol = insertPos + len(vh.yankBuffer) - 1
		}
	}

	vh.editor.isDirty = true
	vh.editor.updateDisplay()
}

func (vh *VimHandler) pasteBefore() {
	if vh.yankBuffer == "" {
		return
	}

	lines := strings.Split(vh.editor.textContent, "\n")

	if strings.HasSuffix(vh.yankBuffer, "\n") {
		// Вставка строки
		newLines := make([]string, 0, len(lines)+1)
		newLines = append(newLines, lines[:vh.editor.cursorRow]...)
		newLines = append(newLines, strings.TrimSuffix(vh.yankBuffer, "\n"))
		newLines = append(newLines, lines[vh.editor.cursorRow:]...)
		vh.editor.textContent = strings.Join(newLines, "\n")
		vh.editor.cursorCol = 0
	} else {
		// Вставка текста
		if vh.editor.cursorRow < len(lines) {
			line := lines[vh.editor.cursorRow]
			lines[vh.editor.cursorRow] = line[:vh.editor.cursorCol] + vh.yankBuffer + line[vh.editor.cursorCol:]
			vh.editor.textContent = strings.Join(lines, "\n")
			vh.editor.cursorCol += len(vh.yankBuffer) - 1
		}
	}

	vh.editor.isDirty = true
	vh.editor.updateDisplay()
}

// Вспомогательные методы

func (vh *VimHandler) startVisualMode() {
	vh.mode = VimVisual
	vh.visualStart = TextPosition{Row: vh.editor.cursorRow, Col: vh.editor.cursorCol}
	vh.visualEnd = vh.visualStart
}

func (vh *VimHandler) startVisualLineMode() {
	vh.mode = VimVisualLine
	vh.visualStart = TextPosition{Row: vh.editor.cursorRow, Col: 0}
	lines := strings.Split(vh.editor.textContent, "\n")
	if vh.editor.cursorRow < len(lines) {
		vh.visualEnd = TextPosition{Row: vh.editor.cursorRow, Col: len(lines[vh.editor.cursorRow])}
	}
}

func (vh *VimHandler) updateSelection() {
	vh.visualEnd = TextPosition{Row: vh.editor.cursorRow, Col: vh.editor.cursorCol}
	vh.editor.selectionStart = vh.visualStart
	vh.editor.selectionEnd = vh.visualEnd
	vh.editor.updateDisplay()
}

func (vh *VimHandler) clearSelection() {
	vh.editor.selectionStart = TextPosition{}
	vh.editor.selectionEnd = TextPosition{}
	vh.editor.updateDisplay()
}

func (vh *VimHandler) deleteSelection() {
	// TODO: Implement delete selection
}

func (vh *VimHandler) yankSelection() {
	// TODO: Implement yank selection
}

func (vh *VimHandler) changeSelection() {
	vh.deleteSelection()
	vh.mode = VimInsert
}

func (vh *VimHandler) changeLine() {
	vh.deleteLine()
	vh.insertLineAbove()
	vh.mode = VimInsert
}

func (vh *VimHandler) changeToLineEnd() {
	vh.deleteToLineEnd()
	vh.mode = VimInsert
}

func (vh *VimHandler) changeWord() {
	vh.deleteWord()
	vh.mode = VimInsert
}

func (vh *VimHandler) insertLineBelow() {
	lines := strings.Split(vh.editor.textContent, "\n")
	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:vh.editor.cursorRow+1]...)
	newLines = append(newLines, "")
	if vh.editor.cursorRow+1 < len(lines) {
		newLines = append(newLines, lines[vh.editor.cursorRow+1:]...)
	}
	vh.editor.textContent = strings.Join(newLines, "\n")
	vh.editor.cursorRow++
	vh.editor.cursorCol = 0
	vh.editor.isDirty = true
	vh.editor.updateDisplay()
}

func (vh *VimHandler) insertLineAbove() {
	lines := strings.Split(vh.editor.textContent, "\n")
	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:vh.editor.cursorRow]...)
	newLines = append(newLines, "")
	newLines = append(newLines, lines[vh.editor.cursorRow:]...)
	vh.editor.textContent = strings.Join(newLines, "\n")
	vh.editor.cursorCol = 0
	vh.editor.isDirty = true
	vh.editor.updateDisplay()
}

func (vh *VimHandler) replaceChar(ch rune) {
	lines := strings.Split(vh.editor.textContent, "\n")
	if vh.editor.cursorRow < len(lines) {
		line := lines[vh.editor.cursorRow]
		if vh.editor.cursorCol < len(line) {
			runes := []rune(line)
			runes[vh.editor.cursorCol] = ch
			lines[vh.editor.cursorRow] = string(runes)
			vh.editor.textContent = strings.Join(lines, "\n")
			vh.editor.isDirty = true
			vh.editor.updateDisplay()
		}
	}
}

func (vh *VimHandler) undo() {
	// TODO: Call editor's undo
}

func (vh *VimHandler) redo() {
	// TODO: Call editor's redo
}

func (vh *VimHandler) repeatLastCommand() {
	if vh.lastCommand != "" {
		vh.executeNormalCommand(vh.lastCommand)
	}
}

func (vh *VimHandler) startSearch(backward bool) {
	vh.searchBackward = backward
	// TODO: Show search prompt
}

func (vh *VimHandler) searchNext() {
	// TODO: Implement search next
}

func (vh *VimHandler) searchPrevious() {
	// TODO: Implement search previous
}

func (vh *VimHandler) searchWordUnderCursor(backward bool) {
	// TODO: Implement search word under cursor
}

func (vh *VimHandler) setMark(mark string) {
	vh.marks[mark] = TextPosition{Row: vh.editor.cursorRow, Col: vh.editor.cursorCol}
}

func (vh *VimHandler) jumpToMark(mark string) {
	if pos, exists := vh.marks[mark]; exists {
		vh.addToJumpList()
		vh.editor.cursorRow = pos.Row
		vh.editor.cursorCol = pos.Col
		vh.editor.updateDisplay()
	}
}

func (vh *VimHandler) addToJumpList() {
	pos := TextPosition{Row: vh.editor.cursorRow, Col: vh.editor.cursorCol}
	vh.jumpList = append(vh.jumpList, pos)
	vh.jumpIndex = len(vh.jumpList) - 1
}

func (vh *VimHandler) jumpToMatchingBracket() {
	// TODO: Implement jump to matching bracket
}

func (vh *VimHandler) scrollHalfPageDown() {
	// TODO: Implement scroll
}

func (vh *VimHandler) scrollHalfPageUp() {
	// TODO: Implement scroll
}

func (vh *VimHandler) scrollPageDown() {
	// TODO: Implement scroll
}

func (vh *VimHandler) scrollPageUp() {
	// TODO: Implement scroll
}

func (vh *VimHandler) startRecordingMacro(register string) {
	vh.recording = true
	vh.macroRegister = register
	vh.macroCommands = []string{}
}

func (vh *VimHandler) stopRecordingMacro() {
	if vh.recording {
		vh.recording = false
		vh.macros[vh.macroRegister] = vh.macroCommands
		vh.macroCommands = nil
	}
}

func (vh *VimHandler) playMacro(register string) {
	if commands, exists := vh.macros[register]; exists {
		for _, cmd := range commands {
			vh.HandleKey(cmd)
		}
	}
}

func (vh *VimHandler) saveFile() {
	if vh.editor != nil {
		vh.editor.SaveFile()
	}
}

func (vh *VimHandler) openFile(filename string) {
	if vh.editor != nil {
		vh.editor.LoadFile(filename)
	}
}

func (vh *VimHandler) quit() {
	// TODO: Implement quit
}

func (vh *VimHandler) forceQuit() {
	// TODO: Implement force quit
}

func (vh *VimHandler) setOption(option string) {
	// TODO: Implement set options (like set number, set wrap, etc)
}

func (vh *VimHandler) substitute(cmd string) {
	// TODO: Implement substitute on current line
}

func (vh *VimHandler) substituteAll(cmd string) {
	// TODO: Implement substitute in whole file
}

// GetMode возвращает текущий режим
func (vh *VimHandler) GetMode() VimMode {
	return vh.mode
}

// GetModeString возвращает строковое представление режима
func (vh *VimHandler) GetModeString() string {
	switch vh.mode {
	case VimNormal:
		return "NORMAL"
	case VimInsert:
		return "INSERT"
	case VimVisual:
		return "VISUAL"
	case VimVisualLine:
		return "V-LINE"
	case VimVisualBlock:
		return "V-BLOCK"
	case VimCommand:
		return "COMMAND"
	case VimReplace:
		return "REPLACE"
	default:
		return ""
	}
}

// Helper functions

func isWordBoundary(ch rune) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' ||
		ch == '.' || ch == ',' || ch == ';' || ch == ':' ||
		ch == '!' || ch == '?' || ch == '"' || ch == '\'' ||
		ch == '(' || ch == ')' || ch == '[' || ch == ']' ||
		ch == '{' || ch == '}' || ch == '<' || ch == '>'
}

func parseLineNumber(s string) int {
	var num int
	fmt.Sscanf(s, "%d", &num)
	return num
}
