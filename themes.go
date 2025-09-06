package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// DarkTheme реализует темную тему в стиле Windows 11
type DarkTheme struct{}

// Windows 11 цветовая палитра (темная)
var (
	// Основные цвета фона
	darkBackground = color.NRGBA{0x20, 0x20, 0x20, 0xFF} // #202020 - основной фон
	darkSurface    = color.NRGBA{0x2D, 0x2D, 0x30, 0xFF} // #2D2D30 - поверхности
	darkCard       = color.NRGBA{0x3C, 0x3C, 0x3C, 0xFF} // #3C3C3C - карточки/панели
	darkElevated   = color.NRGBA{0x48, 0x48, 0x4D, 0xFF} // #48484D - поднятые элементы

	// Акцентные цвета
	accentBlue        = color.NRGBA{0x00, 0x78, 0xD4, 0xFF} // #0078D4 - Windows 11 accent
	accentBlueHover   = color.NRGBA{0x10, 0x88, 0xE4, 0xFF} // #1088E4 - hover состояние
	accentBluePressed = color.NRGBA{0x00, 0x68, 0xC4, 0xFF} // #0068C4 - pressed состояние

	// Текст
	textPrimary     = color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF} // #FFFFFF - основной текст
	textSecondary   = color.NRGBA{0xB0, 0xB0, 0xB0, 0xFF} // #B0B0B0 - вторичный текст
	textDisabled    = color.NRGBA{0x6D, 0x6D, 0x6D, 0xFF} // #6D6D6D - отключенный текст
	textPlaceholder = color.NRGBA{0x86, 0x86, 0x86, 0xFF} // #868686 - placeholder

	// Границы и разделители
	borderDefault = color.NRGBA{0x5A, 0x5A, 0x5A, 0xFF} // #5A5A5A - обычные границы
	borderFocus   = color.NRGBA{0x00, 0x78, 0xD4, 0xFF} // #0078D4 - фокус
	borderHover   = color.NRGBA{0x73, 0x73, 0x73, 0xFF} // #737373 - hover

	// Состояния
	errorColor   = color.NRGBA{0xD1, 0x3B, 0x38, 0xFF} // #D13B38 - ошибки
	warningColor = color.NRGBA{0xFF, 0xB9, 0x00, 0xFF} // #FFB900 - предупреждения
	successColor = color.NRGBA{0x00, 0xAD, 0x56, 0xFF} // #00AD56 - успех

	// Редактор специфичные цвета
	editorBackground    = color.NRGBA{0x1E, 0x1E, 0x1E, 0xFF} // #1E1E1E - фон редактора
	editorGutter        = color.NRGBA{0x25, 0x25, 0x25, 0xFF} // #252525 - gutter для номеров строк
	editorSelection     = color.NRGBA{0x26, 0x4F, 0x78, 0x80} // #264F78 с прозрачностью - выделение
	editorCursor        = color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF} // #FFFFFF - курсор
	editorLineHighlight = color.NRGBA{0x2A, 0x2A, 0x2A, 0xFF} // #2A2A2A - подсветка текущей строки

	// Подсветка синтаксиса
	syntaxKeyword  = color.NRGBA{0x56, 0x9C, 0xD6, 0xFF} // #569CD6 - ключевые слова
	syntaxString   = color.NRGBA{0xCE, 0x91, 0x78, 0xFF} // #CE9178 - строки
	syntaxComment  = color.NRGBA{0x6A, 0x99, 0x55, 0xFF} // #6A9955 - комментарии
	syntaxNumber   = color.NRGBA{0xB5, 0xCE, 0xA8, 0xFF} // #B5CEA8 - числа
	syntaxFunction = color.NRGBA{0xDC, 0xDC, 0xAA, 0xFF} // #DCDCAA - функции
	syntaxType     = color.NRGBA{0x4E, 0xC9, 0xB0, 0xFF} // #4EC9B0 - типы данных
	syntaxVariable = color.NRGBA{0x9C, 0xDC, 0xFE, 0xFF} // #9CDCFE - переменные

	// Специальные элементы UI
	scrollbarTrack    = color.NRGBA{0x2D, 0x2D, 0x30, 0xFF} // #2D2D30 - трек скроллбара
	scrollbarThumb    = color.NRGBA{0x5A, 0x5A, 0x5A, 0xFF} // #5A5A5A - бегунок скроллбара
	tooltipBackground = color.NRGBA{0x25, 0x25, 0x25, 0xF0} // #252525 - фон тултипов
)

// Color возвращает цвета для темной темы
func (t *DarkTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	// Основные цвета фона
	case theme.ColorNameBackground:
		return darkBackground
	case theme.ColorNameForeground:
		return textPrimary
	case theme.ColorNameDisabled:
		return textDisabled
	case theme.ColorNamePlaceHolder:
		return textPlaceholder

	// Элементы управления
	case theme.ColorNamePrimary:
		return accentBlue
	case theme.ColorNameHover:
		return accentBlueHover
	case theme.ColorNamePressed:
		return accentBluePressed
	case theme.ColorNameFocus:
		return borderFocus

	// Поверхности и контейнеры
	case theme.ColorNameInputBackground:
		return darkSurface
	case theme.ColorNameButton:
		return darkCard
	case theme.ColorNameHeaderBackground:
		return darkElevated
	case theme.ColorNameMenuBackground:
		return darkCard
	case theme.ColorNameOverlayBackground:
		return tooltipBackground

	// Границы
	case theme.ColorNameSeparator:
		return borderDefault
	case theme.ColorNameSelection:
		return editorSelection

	// Состояния
	case theme.ColorNameError:
		return errorColor
	case theme.ColorNameWarning:
		return warningColor
	case theme.ColorNameSuccess:
		return successColor

	// Скроллбары
	case theme.ColorNameScrollBar:
		return scrollbarThumb

	// Тени (для Windows 11 эффектов)
	case theme.ColorNameShadow:
		return color.NRGBA{0x00, 0x00, 0x00, 0x40}

	default:
		// Fallback на стандартную темную тему
		return theme.DefaultTheme().Color(name, variant)
	}
}

// Font возвращает шрифты в стиле Windows 11
func (t *DarkTheme) Font(style fyne.TextStyle) fyne.Resource {
	if style.Monospace {
		// Моноширинный шрифт для кода (будет JetBrains Mono из конфига)
		return theme.DefaultTheme().Font(style)
	}
	// Основной шрифт UI (будет Segoe UI Variable из системы)
	return theme.DefaultTheme().Font(style)
}

// Icon возвращает иконки с поддержкой темной темы
func (t *DarkTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	// Используем стандартные иконки, но можем кастомизировать
	switch name {
	case theme.IconNameFolder:
		return theme.FolderIcon()
	case theme.IconNameFile:
		return theme.DocumentIcon()
	case theme.IconNameComputer:
		return theme.ComputerIcon()
	case theme.IconNameStorage:
		return theme.StorageIcon()
	default:
		return theme.DefaultTheme().Icon(name)
	}
}

// Size возвращает размеры элементов в стиле Windows 11
func (t *DarkTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 8 // Увеличенные отступы для современного вида
	case theme.SizeNameInlineIcon:
		return 16
	case theme.SizeNameScrollBar:
		return 12 // Тонкие скроллбары как в Windows 11
	case theme.SizeNameSeparatorThickness:
		return 1 // Тонкие разделители
	case theme.SizeNameInputBorder:
		return 1 // Тонкие границы полей ввода
	default:
		return theme.DefaultTheme().Size(name)
	}
}

// LightTheme реализует светлую тему в стиле Windows 11
type LightTheme struct{}

// Светлая цветовая палитра Windows 11
var (
	lightBackground = color.NRGBA{0xF9, 0xF9, 0xF9, 0xFF} // #F9F9F9
	lightSurface    = color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF} // #FFFFFF
	lightCard       = color.NRGBA{0xF0, 0xF0, 0xF0, 0xFF} // #F0F0F0
	lightElevated   = color.NRGBA{0xE6, 0xE6, 0xE6, 0xFF} // #E6E6E6

	lightTextPrimary   = color.NRGBA{0x32, 0x32, 0x32, 0xFF} // #323232
	lightTextSecondary = color.NRGBA{0x60, 0x60, 0x60, 0xFF} // #606060
	lightTextDisabled  = color.NRGBA{0xB0, 0xB0, 0xB0, 0xFF} // #B0B0B0

	lightBorderDefault = color.NRGBA{0xD1, 0xD1, 0xD1, 0xFF} // #D1D1D1

	lightEditorBackground = color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF} // #FFFFFF
	lightEditorGutter     = color.NRGBA{0xF5, 0xF5, 0xF5, 0xFF} // #F5F5F5
	lightEditorSelection  = color.NRGBA{0x00, 0x78, 0xD4, 0x40} // #0078D4 с прозрачностью
)

// Color для светлой темы
func (t *LightTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return lightBackground
	case theme.ColorNameForeground:
		return lightTextPrimary
	case theme.ColorNameDisabled:
		return lightTextDisabled
	case theme.ColorNamePrimary:
		return accentBlue
	case theme.ColorNameInputBackground:
		return lightSurface
	case theme.ColorNameButton:
		return lightCard
	case theme.ColorNameHeaderBackground:
		return lightElevated
	case theme.ColorNameSeparator:
		return lightBorderDefault
	case theme.ColorNameSelection:
		return lightEditorSelection
	default:
		return theme.DefaultTheme().Color(name, variant)
	}
}

// Font для светлой темы (аналогично темной)
func (t *LightTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

// Icon для светлой темы
func (t *LightTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

// Size для светлой темы (аналогично темной)
func (t *LightTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 8
	case theme.SizeNameScrollBar:
		return 12
	case theme.SizeNameSeparatorThickness:
		return 1
	case theme.SizeNameInputBorder:
		return 1
	default:
		return theme.DefaultTheme().Size(name)
	}
}

// GetEditorColors возвращает специфичные цвета для редактора
func GetEditorColors(isDark bool) EditorColors {
	if isDark {
		return EditorColors{
			Background:    editorBackground,
			Gutter:        editorGutter,
			Selection:     editorSelection,
			Cursor:        editorCursor,
			LineHighlight: editorLineHighlight,
			LineNumbers:   textSecondary,
			Keyword:       syntaxKeyword,
			String:        syntaxString,
			Comment:       syntaxComment,
			Number:        syntaxNumber,
			Function:      syntaxFunction,
			Type:          syntaxType,
			Variable:      syntaxVariable,
		}
	} else {
		return EditorColors{
			Background:    lightEditorBackground,
			Gutter:        lightEditorGutter,
			Selection:     lightEditorSelection,
			Cursor:        lightTextPrimary,
			LineHighlight: color.NRGBA{0xF8, 0xF8, 0xF8, 0xFF},
			LineNumbers:   lightTextSecondary,
			Keyword:       color.NRGBA{0x00, 0x00, 0xFF, 0xFF}, // Синие ключевые слова
			String:        color.NRGBA{0xA3, 0x15, 0x15, 0xFF}, // Красные строки
			Comment:       color.NRGBA{0x00, 0x80, 0x00, 0xFF}, // Зеленые комментарии
			Number:        color.NRGBA{0x09, 0x88, 0x5A, 0xFF}, // Темно-зеленые числа
			Function:      color.NRGBA{0x79, 0x5E, 0x26, 0xFF}, // Коричневые функции
			Type:          color.NRGBA{0x26, 0x7F, 0x99, 0xFF}, // Сине-зеленые типы
			Variable:      lightTextPrimary,                    // Обычный текст
		}
	}
}

// EditorColors содержит цвета для редактора кода
type EditorColors struct {
	Background    color.Color
	Gutter        color.Color
	Selection     color.Color
	Cursor        color.Color
	LineHighlight color.Color
	LineNumbers   color.Color
	Keyword       color.Color
	String        color.Color
	Comment       color.Color
	Number        color.Color
	Function      color.Color
	Type          color.Color
	Variable      color.Color
}
