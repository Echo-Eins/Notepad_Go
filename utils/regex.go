package main

import (
	"fmt"
	"regexp"
	"strings"
)

// RegexMatcher предоставляет утилиты для работы с регулярными выражениями
type RegexMatcher struct {
	patterns map[string]*CompiledPattern
	cache    map[string]*regexp.Regexp
}

// CompiledPattern представляет скомпилированный паттерн
type CompiledPattern struct {
	Name        string
	Pattern     *regexp.Regexp
	Description string
	Groups      []string
}

// NewRegexMatcher создает новый matcher
func NewRegexMatcher() *RegexMatcher {
	rm := &RegexMatcher{
		patterns: make(map[string]*CompiledPattern),
		cache:    make(map[string]*regexp.Regexp),
	}

	// Загружаем стандартные паттерны
	rm.loadStandardPatterns()

	return rm
}

// loadStandardPatterns загружает стандартные паттерны для языков программирования
func (rm *RegexMatcher) loadStandardPatterns() {
	// Go patterns
	rm.AddPattern("go_import", `import\s+(?:"([^"]+)"|`+"`"+`([^`+"`"+`]+)`+"`"+`)`,
		"Go import statement", []string{"package"})
	rm.AddPattern("go_function", `func\s+(?:\([^)]+\)\s+)?(\w+)\s*\([^)]*\)`,
		"Go function declaration", []string{"name"})
	rm.AddPattern("go_struct", `type\s+(\w+)\s+struct\s*\{`,
		"Go struct declaration", []string{"name"})
	rm.AddPattern("go_interface", `type\s+(\w+)\s+interface\s*\{`,
		"Go interface declaration", []string{"name"})
	rm.AddPattern("go_error", `^([^:]+):(\d+):(\d+):\s*(.+)$`,
		"Go compiler error", []string{"file", "line", "column", "message"})

	// Python patterns
	rm.AddPattern("python_import", `(?:from\s+(\S+)\s+)?import\s+(.+)`,
		"Python import statement", []string{"module", "names"})
	rm.AddPattern("python_function", `def\s+(\w+)\s*\([^)]*\):`,
		"Python function declaration", []string{"name"})
	rm.AddPattern("python_class", `class\s+(\w+)(?:\([^)]*\))?:`,
		"Python class declaration", []string{"name"})
	rm.AddPattern("python_error", `File\s+"([^"]+)",\s+line\s+(\d+)(?:,\s+in\s+(\w+))?`,
		"Python traceback", []string{"file", "line", "function"})

	// Rust patterns
	rm.AddPattern("rust_function", `fn\s+(\w+)\s*(?:<[^>]+>)?\s*\([^)]*\)`,
		"Rust function declaration", []string{"name"})
	rm.AddPattern("rust_struct", `struct\s+(\w+)(?:<[^>]+>)?\s*[{;]`,
		"Rust struct declaration", []string{"name"})
	rm.AddPattern("rust_impl", `impl(?:<[^>]+>)?\s+(?:(\w+)\s+for\s+)?(\w+)`,
		"Rust impl block", []string{"trait", "type"})
	rm.AddPattern("rust_error", `error(?:\[(\w+)\])?: (.+)\s+--> ([^:]+):(\d+):(\d+)`,
		"Rust compiler error", []string{"code", "message", "file", "line", "column"})

	// C/C++ patterns
	rm.AddPattern("c_function", `(?:\w+\s+)*(\w+)\s*\([^)]*\)\s*\{`,
		"C function declaration", []string{"name"})
	rm.AddPattern("c_include", `#include\s+[<"]([^>"]+)[>"]`,
		"C include directive", []string{"file"})
	rm.AddPattern("c_define", `#define\s+(\w+)(?:\([^)]*\))?\s+(.*)`,
		"C define directive", []string{"name", "value"})
	rm.AddPattern("c_error", `([^:]+):(\d+):(\d+):\s*(error|warning):\s*(.+)`,
		"C compiler message", []string{"file", "line", "column", "type", "message"})

	// Java patterns
	rm.AddPattern("java_class", `(?:public\s+)?class\s+(\w+)(?:\s+extends\s+(\w+))?(?:\s+implements\s+([^{]+))?`,
		"Java class declaration", []string{"name", "extends", "implements"})
	rm.AddPattern("java_method", `(?:public|private|protected)?\s*(?:static\s+)?(?:\w+\s+)?(\w+)\s*\([^)]*\)`,
		"Java method declaration", []string{"name"})
	rm.AddPattern("java_import", `import\s+(?:static\s+)?([^;]+);`,
		"Java import statement", []string{"package"})
	rm.AddPattern("java_error", `([^:]+):(\d+):\s*(error|warning):\s*(.+)`,
		"Java compiler message", []string{"file", "line", "type", "message"})

	// Common patterns
	rm.AddPattern("url", `https?://[^\s<>"{}|\\^`+"`"+`\[\]]+`,
		"URL", []string{})
	rm.AddPattern("email", `\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`,
		"Email address", []string{})
	rm.AddPattern("ip_address", `\b(?:\d{1,3}\.){3}\d{1,3}\b`,
		"IP address", []string{})
	rm.AddPattern("hex_color", `#[0-9A-Fa-f]{6}\b|#[0-9A-Fa-f]{3}\b`,
		"Hex color code", []string{})
	rm.AddPattern("todo_comment", `(?://|#|/\*)\s*(TODO|FIXME|NOTE|HACK|BUG):\s*(.*)`,
		"TODO comment", []string{"type", "message"})
	rm.AddPattern("file_path_windows", `[A-Za-z]:\\(?:[^\\/:*?"<>|\r\n]+\\)*[^\\/:*?"<>|\r\n]*`,
		"Windows file path", []string{})
	rm.AddPattern("file_path_unix", `/(?:[^/\0]+/)*[^/\0]*`,
		"Unix file path", []string{})
}

// AddPattern добавляет новый паттерн
func (rm *RegexMatcher) AddPattern(name, pattern, description string, groups []string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("failed to compile pattern '%s': %v", name, err)
	}

	rm.patterns[name] = &CompiledPattern{
		Name:        name,
		Pattern:     re,
		Description: description,
		Groups:      groups,
	}

	return nil
}

// Match выполняет поиск по паттерну
func (rm *RegexMatcher) Match(patternName, text string) ([]RegexMatch, error) {
	pattern, exists := rm.patterns[patternName]
	if !exists {
		return nil, fmt.Errorf("pattern '%s' not found", patternName)
	}

	return rm.findMatches(pattern, text), nil
}

// MatchCustom выполняет поиск по пользовательскому паттерну
func (rm *RegexMatcher) MatchCustom(patternStr, text string) ([]RegexMatch, error) {
	// Проверяем кэш
	re, exists := rm.cache[patternStr]
	if !exists {
		var err error
		re, err = regexp.Compile(patternStr)
		if err != nil {
			return nil, fmt.Errorf("invalid regex pattern: %v", err)
		}
		rm.cache[patternStr] = re
	}

	pattern := &CompiledPattern{
		Pattern: re,
	}

	return rm.findMatches(pattern, text), nil
}

// findMatches находит все совпадения
func (rm *RegexMatcher) findMatches(pattern *CompiledPattern, text string) []RegexMatch {
	var matches []RegexMatch

	allMatches := pattern.Pattern.FindAllStringSubmatchIndex(text, -1)
	for _, match := range allMatches {
		if len(match) < 2 {
			continue
		}

		regexMatch := RegexMatch{
			Text:       text[match[0]:match[1]],
			Start:      match[0],
			End:        match[1],
			Groups:     make(map[string]string),
			GroupsList: make([]string, 0),
		}

		// Извлекаем группы
		for i := 2; i < len(match); i += 2 {
			if match[i] != -1 && match[i+1] != -1 {
				groupText := text[match[i]:match[i+1]]
				regexMatch.GroupsList = append(regexMatch.GroupsList, groupText)

				// Если есть именованные группы
				if pattern.Groups != nil && (i/2-1) < len(pattern.Groups) {
					groupName := pattern.Groups[i/2-1]
					regexMatch.Groups[groupName] = groupText
				}
			}
		}

		matches = append(matches, regexMatch)
	}

	return matches
}

// Replace выполняет замену по паттерну
func (rm *RegexMatcher) Replace(patternName, text, replacement string) (string, error) {
	pattern, exists := rm.patterns[patternName]
	if !exists {
		return "", fmt.Errorf("pattern '%s' not found", patternName)
	}

	return pattern.Pattern.ReplaceAllString(text, replacement), nil
}

// ReplaceCustom выполняет замену по пользовательскому паттерну
func (rm *RegexMatcher) ReplaceCustom(patternStr, text, replacement string) (string, error) {
	re, exists := rm.cache[patternStr]
	if !exists {
		var err error
		re, err = regexp.Compile(patternStr)
		if err != nil {
			return "", fmt.Errorf("invalid regex pattern: %v", err)
		}
		rm.cache[patternStr] = re
	}

	return re.ReplaceAllString(text, replacement), nil
}

// RegexMatch представляет совпадение
type RegexMatch struct {
	Text       string            // Полный текст совпадения
	Start      int               // Начальная позиция
	End        int               // Конечная позиция
	Groups     map[string]string // Именованные группы
	GroupsList []string          // Список групп по порядку
	Line       int               // Номер строки (если применимо)
	Column     int               // Номер колонки (если применимо)
}

// CodeAnalyzer анализирует код с помощью регулярных выражений
type CodeAnalyzer struct {
	matcher *RegexMatcher
}

// NewCodeAnalyzer создает новый анализатор кода
func NewCodeAnalyzer() *CodeAnalyzer {
	return &CodeAnalyzer{
		matcher: NewRegexMatcher(),
	}
}

// FindFunctions находит объявления функций в коде
func (ca *CodeAnalyzer) FindFunctions(code, language string) []CodeElement {
	patternName := language + "_function"
	matches, err := ca.matcher.Match(patternName, code)
	if err != nil {
		return nil
	}

	elements := make([]CodeElement, 0, len(matches))
	for _, match := range matches {
		element := CodeElement{
			Type:     "function",
			Name:     match.Groups["name"],
			Position: match.Start,
			Text:     match.Text,
		}

		// Определяем строку и колонку
		element.Line, element.Column = ca.getLineColumn(code, match.Start)

		elements = append(elements, element)
	}

	return elements
}

// FindClasses находит объявления классов/структур в коде
func (ca *CodeAnalyzer) FindClasses(code, language string) []CodeElement {
	var patternName string
	elementType := "class"

	switch language {
	case "go":
		patternName = "go_struct"
		elementType = "struct"
	case "rust":
		patternName = "rust_struct"
		elementType = "struct"
	case "python":
		patternName = "python_class"
	case "java":
		patternName = "java_class"
	default:
		return nil
	}

	matches, err := ca.matcher.Match(patternName, code)
	if err != nil {
		return nil
	}

	elements := make([]CodeElement, 0, len(matches))
	for _, match := range matches {
		element := CodeElement{
			Type:     elementType,
			Name:     match.Groups["name"],
			Position: match.Start,
			Text:     match.Text,
		}

		element.Line, element.Column = ca.getLineColumn(code, match.Start)
		elements = append(elements, element)
	}

	return elements
}

// FindImports находит импорты в коде
func (ca *CodeAnalyzer) FindImports(code, language string) []CodeElement {
	patternName := language + "_import"
	matches, err := ca.matcher.Match(patternName, code)
	if err != nil {
		return nil
	}

	elements := make([]CodeElement, 0, len(matches))
	for _, match := range matches {
		importPath := match.Groups["package"]
		if importPath == "" && len(match.GroupsList) > 0 {
			importPath = match.GroupsList[0]
		}

		element := CodeElement{
			Type:     "import",
			Name:     importPath,
			Position: match.Start,
			Text:     match.Text,
		}

		element.Line, element.Column = ca.getLineColumn(code, match.Start)
		elements = append(elements, element)
	}

	return elements
}

// FindErrors находит сообщения об ошибках в выводе компилятора
func (ca *CodeAnalyzer) FindErrors(output, language string) []CompilerError {
	patternName := language + "_error"
	matches, err := ca.matcher.Match(patternName, output)
	if err != nil {
		return nil
	}

	errors := make([]CompilerError, 0, len(matches))
	for _, match := range matches {
		compErr := CompilerError{
			File:    match.Groups["file"],
			Message: match.Groups["message"],
			Text:    match.Text,
		}

		// Парсим номер строки
		if lineStr := match.Groups["line"]; lineStr != "" {
			if line, err := parseInt(lineStr); err == nil {
				compErr.Line = line
			}
		}

		// Парсим номер колонки
		if colStr := match.Groups["column"]; colStr != "" {
			if col, err := parseInt(colStr); err == nil {
				compErr.Column = col
			}
		}

		// Тип ошибки
		if errType := match.Groups["type"]; errType != "" {
			compErr.Type = errType
		} else {
			compErr.Type = "error"
		}

		errors = append(errors, compErr)
	}

	return errors
}

// FindTODOs находит TODO комментарии
func (ca *CodeAnalyzer) FindTODOs(code string) []TodoComment {
	matches, err := ca.matcher.Match("todo_comment", code)
	if err != nil {
		return nil
	}

	todos := make([]TodoComment, 0, len(matches))
	for _, match := range matches {
		todo := TodoComment{
			Type:     match.Groups["type"],
			Message:  match.Groups["message"],
			Position: match.Start,
			Text:     match.Text,
		}

		todo.Line, todo.Column = ca.getLineColumn(code, match.Start)
		todos = append(todos, todo)
	}

	return todos
}

// FindURLs находит URL в тексте
func (ca *CodeAnalyzer) FindURLs(text string) []string {
	matches, err := ca.matcher.Match("url", text)
	if err != nil {
		return nil
	}

	urls := make([]string, 0, len(matches))
	for _, match := range matches {
		urls = append(urls, match.Text)
	}

	return urls
}

// FindColorCodes находит цветовые коды
func (ca *CodeAnalyzer) FindColorCodes(text string) []ColorCode {
	matches, err := ca.matcher.Match("hex_color", text)
	if err != nil {
		return nil
	}

	colors := make([]ColorCode, 0, len(matches))
	for _, match := range matches {
		color := ColorCode{
			Code:     match.Text,
			Position: match.Start,
		}

		color.Line, color.Column = ca.getLineColumn(text, match.Start)
		colors = append(colors, color)
	}

	return colors
}

// getLineColumn определяет номер строки и колонки по позиции
func (ca *CodeAnalyzer) getLineColumn(text string, position int) (line, column int) {
	if position < 0 || position > len(text) {
		return 0, 0
	}

	line = 1
	column = 1

	for i := 0; i < position; i++ {
		if text[i] == '\n' {
			line++
			column = 1
		} else {
			column++
		}
	}

	return line, column
}

// CodeElement представляет элемент кода
type CodeElement struct {
	Type     string // function, class, struct, import, etc.
	Name     string
	Position int
	Line     int
	Column   int
	Text     string
}

// CompilerError представляет ошибку компилятора
type CompilerError struct {
	File    string
	Line    int
	Column  int
	Type    string // error, warning
	Message string
	Text    string
}

// TodoComment представляет TODO комментарий
type TodoComment struct {
	Type     string // TODO, FIXME, NOTE, etc.
	Message  string
	Position int
	Line     int
	Column   int
	Text     string
}

// ColorCode представляет цветовой код
type ColorCode struct {
	Code     string
	Position int
	Line     int
	Column   int
}

// ParameterHintExtractor извлекает подсказки параметров
type ParameterHintExtractor struct {
	matcher *RegexMatcher
}

// NewParameterHintExtractor создает новый экстрактор подсказок
func NewParameterHintExtractor() *ParameterHintExtractor {
	return &ParameterHintExtractor{
		matcher: NewRegexMatcher(),
	}
}

// ExtractFunctionSignature извлекает сигнатуру функции
func (phe *ParameterHintExtractor) ExtractFunctionSignature(line string, language string) *FunctionSignature {
	// Паттерны для извлечения сигнатур функций
	patterns := map[string]string{
		"go":     `(\w+)\s*\(([^)]*)\)(?:\s*(?:\(([^)]*)\)|\w+))?`,
		"python": `(\w+)\s*\(([^)]*)\)`,
		"rust":   `(\w+)\s*(?:<[^>]+>)?\s*\(([^)]*)\)(?:\s*->\s*(.+))?`,
		"c":      `(?:\w+\s+)*(\w+)\s*\(([^)]*)\)`,
		"java":   `(?:\w+\s+)?(\w+)\s*\(([^)]*)\)`,
	}

	pattern, exists := patterns[language]
	if !exists {
		return nil
	}

	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(line)
	if len(matches) < 3 {
		return nil
	}

	sig := &FunctionSignature{
		Name:       matches[1],
		Parameters: phe.parseParameters(matches[2], language),
	}

	// Возвращаемый тип (если есть)
	if len(matches) > 3 && matches[3] != "" {
		sig.ReturnType = strings.TrimSpace(matches[3])
	}

	return sig
}

// parseParameters парсит параметры функции
func (phe *ParameterHintExtractor) parseParameters(paramsStr string, language string) []Parameter {
	if strings.TrimSpace(paramsStr) == "" {
		return nil
	}

	var params []Parameter

	// Разделяем параметры по запятым (учитывая вложенные типы)
	paramParts := phe.splitParameters(paramsStr)

	for _, part := range paramParts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		param := Parameter{Text: part}

		// Парсим имя и тип в зависимости от языка
		switch language {
		case "go":
			// name type или name, name2 type
			parts := strings.Fields(part)
			if len(parts) >= 2 {
				param.Name = parts[0]
				param.Type = strings.Join(parts[1:], " ")
			}

		case "python":
			// name или name: type или name = default
			if idx := strings.Index(part, ":"); idx != -1 {
				param.Name = strings.TrimSpace(part[:idx])
				remaining := strings.TrimSpace(part[idx+1:])
				if defIdx := strings.Index(remaining, "="); defIdx != -1 {
					param.Type = strings.TrimSpace(remaining[:defIdx])
					param.DefaultValue = strings.TrimSpace(remaining[defIdx+1:])
				} else {
					param.Type = remaining
				}
			} else if idx := strings.Index(part, "="); idx != -1 {
				param.Name = strings.TrimSpace(part[:idx])
				param.DefaultValue = strings.TrimSpace(part[idx+1:])
			} else {
				param.Name = part
			}

		case "c", "java":
			// type name
			parts := strings.Fields(part)
			if len(parts) >= 2 {
				param.Type = strings.Join(parts[:len(parts)-1], " ")
				param.Name = parts[len(parts)-1]
			}

		case "rust":
			// name: type или mut name: type
			if strings.HasPrefix(part, "mut ") {
				part = part[4:]
			}
			if idx := strings.Index(part, ":"); idx != -1 {
				param.Name = strings.TrimSpace(part[:idx])
				param.Type = strings.TrimSpace(part[idx+1:])
			}
		}

		params = append(params, param)
	}

	return params
}

// splitParameters разделяет параметры с учетом вложенных типов
func (phe *ParameterHintExtractor) splitParameters(paramsStr string) []string {
	var params []string
	var current strings.Builder
	depth := 0

	for _, ch := range paramsStr {
		switch ch {
		case '<', '(', '[', '{':
			depth++
			current.WriteRune(ch)
		case '>', ')', ']', '}':
			depth--
			current.WriteRune(ch)
		case ',':
			if depth == 0 {
				params = append(params, current.String())
				current.Reset()
			} else {
				current.WriteRune(ch)
			}
		default:
			current.WriteRune(ch)
		}
	}

	if current.Len() > 0 {
		params = append(params, current.String())
	}

	return params
}

// FunctionSignature представляет сигнатуру функции
type FunctionSignature struct {
	Name       string
	Parameters []Parameter
	ReturnType string
}

// Parameter представляет параметр функции
type Parameter struct {
	Name         string
	Type         string
	DefaultValue string
	Text         string // Полный текст параметра
}

// Helper functions

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
