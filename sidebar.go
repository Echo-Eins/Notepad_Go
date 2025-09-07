package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/fsnotify/fsnotify"
)

// SidebarWidget - полноценный файловый менеджер
type SidebarWidget struct {
	widget.BaseWidget

	// UI компоненты
	mainContainer *fyne.Container
	toolbar       *fyne.Container
	searchEntry   *widget.Entry
	filterSelect  *widget.Select
	fileTree      *widget.Tree
	statusLabel   *widget.Label

	lastTap     string
	lastTapTime time.Time

	// Состояние
	rootPath    string
	currentPath string
	isVisible   bool
	isExpanded  bool

	// Данные файлов
	fileNodes     map[string]*FileNode
	filteredNodes map[string]*FileNode
	searchResults []string
	selectedFile  string

	// Фильтрация и поиск
	activeFilter    string
	searchTerm      string
	showHiddenFiles bool
	sortBy          SortType
	sortAscending   bool

	// File watching
	watcher       *fsnotify.Watcher
	watcherCancel context.CancelFunc
	watchedDirs   map[string]bool

	// Производительность
	loadMutex    sync.RWMutex
	refreshChan  chan string
	isRefreshing bool

	// Настройки
	config *Config

	// Callbacks
	onFileSelected func(string)
	onFileOpened   func(string)
	onPathChanged  func(string)
}

// FileNode представляет узел в дереве файлов
type FileNode struct {
	Path        string
	Name        string
	IsDir       bool
	Size        int64
	Modified    time.Time
	Extension   string
	Icon        fyne.Resource
	Children    []*FileNode
	IsExpanded  bool
	IsLoaded    bool
	Parent      *FileNode
	Permissions os.FileMode
	IsHidden    bool
}

// SortType определяет тип сортировки
type SortType int

const (
	SortByName SortType = iota
	SortByType
	SortBySize
	SortByDate
)

// FileFilter определяет фильтр файлов
type FileFilter struct {
	Name       string
	Extensions []string
	Pattern    *regexp.Regexp
}

// Предопределенные фильтры файлов
var FileFilters = map[string]FileFilter{
	"All": {
		Name:       "All Files",
		Extensions: []string{"*"},
		Pattern:    regexp.MustCompile(".*"),
	},
	"Code": {
		Name:       "Code Files",
		Extensions: []string{".go", ".py", ".rs", ".c", ".h", ".java", ".js", ".ts"},
		Pattern:    regexp.MustCompile(`\.(go|py|rs|c|h|java|js|ts)$`),
	},
	"Go": {
		Name:       "Go Files",
		Extensions: []string{".go"},
		Pattern:    regexp.MustCompile(`\.go$`),
	},
	"Python": {
		Name:       "Python Files",
		Extensions: []string{".py"},
		Pattern:    regexp.MustCompile(`\.py$`),
	},
	"Rust": {
		Name:       "Rust Files",
		Extensions: []string{".rs"},
		Pattern:    regexp.MustCompile(`\.rs$`),
	},
	"C/C++": {
		Name:       "C/C++ Files",
		Extensions: []string{".c", ".h", ".cpp", ".hpp"},
		Pattern:    regexp.MustCompile(`\.(c|h|cpp|hpp)$`),
	},
	"Java": {
		Name:       "Java Files",
		Extensions: []string{".java"},
		Pattern:    regexp.MustCompile(`\.java$`),
	},
	"Text": {
		Name:       "Text Files",
		Extensions: []string{".txt", ".md", ".json", ".xml", ".yaml", ".yml"},
		Pattern:    regexp.MustCompile(`\.(txt|md|json|xml|yaml|yml)$`),
	},
}

// setupComponents создает UI компоненты
func (s *SidebarWidget) setupComponents() {
	// Поиск
	s.searchEntry = widget.NewEntry()
	s.searchEntry.SetPlaceHolder("Search files...")
	s.searchEntry.OnChanged = func(text string) {
		s.searchTerm = text
		s.applyFilterAndSearch()
	}

	// Фильтр по типам файлов
	filterOptions := []string{}
	for key := range FileFilters {
		filterOptions = append(filterOptions, key)
	}
	sort.Strings(filterOptions)

	s.filterSelect = widget.NewSelect(filterOptions, func(selected string) {
		s.activeFilter = selected
		s.applyFilterAndSearch()
	})

	// Создаем дерево файлов
	s.fileTree = widget.NewTree(
		s.treeChildUIDs,
		s.treeIsBranch,
		s.treeCreateNode,
		s.treeUpdateNode,
	)

	// Обработчики событий дерева
	s.fileTree.OnSelected = func(uid string) {
		now := time.Now()
		if s.lastTap == uid && now.Sub(s.lastTapTime) < 400*time.Millisecond {
			// двойной клик
			node := s.getNodeByPath(uid)
			if node != nil && !node.IsDir {
				if s.onFileOpened != nil {
					s.onFileOpened(uid)
				}
			}
		} else {
			// обычный клик
			s.selectedFile = uid
			if s.onFileSelected != nil {
				s.onFileSelected(uid)
			}
		}
		s.lastTap = uid
		s.lastTapTime = now
	}

	// Инициализация фильтра по умолчанию после создания дерева
	s.filterSelect.SetSelected("All")

	// Кнопки управления
	refreshBtn := NewAnimatedButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		s.RefreshPath(s.currentPath)
	})
	refreshBtn.SetText("Refresh")

	upBtn := NewAnimatedButtonWithIcon("", theme.NavigateBackIcon(), func() {
		s.NavigateUp()
	})
	upBtn.SetText("Up")

	homeBtn := NewAnimatedButtonWithIcon("", theme.HomeIcon(), func() {
		s.NavigateToHome()
	})
	homeBtn.SetText("Home")

	// Настройки отображения
	settingsBtn := NewAnimatedButtonWithIcon("", theme.SettingsIcon(), func() {
		s.showSettingsDialog()
	})

	// Toolbar
	s.toolbar = container.NewHBox(
		upBtn, homeBtn, refreshBtn, settingsBtn,
	)

	// Статус бар
	s.statusLabel = widget.NewLabel("Ready")
	s.statusLabel.TextStyle.Italic = true

	// Toolbar + строка поиска
	searchContainer := container.NewBorder(nil, nil, nil, s.filterSelect, s.searchEntry)

	// Основной layout
	s.mainContainer = container.NewBorder(
		container.NewVBox(s.toolbar, searchContainer), // top
		s.statusLabel,                   // bottom
		nil,                             // left
		nil,                             // right
		container.NewScroll(s.fileTree), // content (center)
	)
}

// NewSidebar создает новый файловый менеджер
func NewSidebar(config *Config) *SidebarWidget {
	sidebar := &SidebarWidget{
		config:        config,
		fileNodes:     make(map[string]*FileNode),
		filteredNodes: make(map[string]*FileNode),
		watchedDirs:   make(map[string]bool),
		refreshChan:   make(chan string, 100),
		isVisible:     true,
		activeFilter:  "All",
		sortBy:        SortByName,
		sortAscending: true,
	}

	sidebar.ExtendBaseWidget(sidebar)
	sidebar.setupComponents()
	sidebar.startRefreshWorker()

	return sidebar
}

// Tree interface methods

// FocusGained is called when the sidebar receives focus. We forward the
// event to the file tree so keyboard navigation continues to work.
func (s *SidebarWidget) FocusGained() {
	if s.fileTree != nil {
		s.fileTree.FocusGained()
	}
}

// FocusLost is called when the sidebar loses focus. Forward to child widgets.
func (s *SidebarWidget) FocusLost() {
	if s.fileTree != nil {
		s.fileTree.FocusLost()
	}
}

// TypedRune forwards rune input to the search entry for quick filtering.
func (s *SidebarWidget) TypedRune(r rune) {
	if s.searchEntry != nil {
		s.searchEntry.TypedRune(r)
	}
}

// TypedKey forwards key events to the file tree for navigation.
func (s *SidebarWidget) TypedKey(event *fyne.KeyEvent) {
	if s.fileTree != nil {
		s.fileTree.TypedKey(event)
	}
}

// treeChildUIDs возвращает дочерние узлы
func (s *SidebarWidget) treeChildUIDs(uid string) []string {
	s.loadMutex.RLock()
	defer s.loadMutex.RUnlock()

	if uid == "" {
		// Корневой узел
		if s.currentPath != "" {
			return []string{s.currentPath}
		}
		return []string{}
	}

	node := s.getNodeByPath(uid)
	if node == nil || !node.IsDir {
		return []string{}
	}

	// Загружаем дочерние узлы если не загружены
	if !node.IsLoaded {
		s.loadDirectoryChildren(node)
	}

	// Применяем фильтры
	var children []string
	for _, child := range node.Children {
		if s.shouldShowNode(child) {
			children = append(children, child.Path)
		}
	}

	// Сортируем
	s.sortPaths(children)

	return children
}

// treeIsBranch определяет, является ли узел веткой
func (s *SidebarWidget) treeIsBranch(uid string) bool {
	node := s.getNodeByPath(uid)
	return node != nil && node.IsDir
}

// treeCreateNode создает виджет для узла
func (s *SidebarWidget) treeCreateNode(branch bool) fyne.CanvasObject {
	icon := widget.NewIcon(theme.DocumentIcon())
	label := widget.NewLabel("Template")

	if branch {
		icon.SetResource(theme.FolderIcon())
	}

	nodeContainer := container.NewHBox(icon, label)
	return nodeContainer
}

// treeUpdateNode обновляет виджет узла
func (s *SidebarWidget) treeUpdateNode(uid string, branch bool, node fyne.CanvasObject) {
	fileNode := s.getNodeByPath(uid)
	if fileNode == nil {
		return
	}

	nodeContainer := node.(*fyne.Container) // вместо *container.HBox
	icon := nodeContainer.Objects[0].(*widget.Icon)
	label := nodeContainer.Objects[1].(*widget.Label)

	// Устанавливаем иконку
	icon.SetResource(s.getNodeIcon(fileNode))

	// Устанавливаем текст
	displayName := fileNode.Name

	// Добавляем информацию о размере для файлов
	if !fileNode.IsDir && s.config != nil {
		sizeStr := s.formatFileSize(fileNode.Size)
		displayName = fmt.Sprintf("%s (%s)", fileNode.Name, sizeStr)
	}

	// Выделяем найденные файлы
	if s.searchTerm != "" && s.matchesSearch(fileNode, s.searchTerm) {
		label.Importance = widget.HighImportance
	} else {
		label.Importance = widget.MediumImportance
	}

	label.SetText(displayName)
}

// SetRootPath устанавливает корневую директорию
func (s *SidebarWidget) SetRootPath(path string) error {
	// Проверяем существование директории
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot access directory: %v", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}

	s.stopWatching()

	s.rootPath = path
	s.currentPath = path
	s.fileNodes = make(map[string]*FileNode)
	s.filteredNodes = make(map[string]*FileNode)

	// Создаем корневой узел
	rootNode := &FileNode{
		Path:     path,
		Name:     filepath.Base(path),
		IsDir:    true,
		Modified: info.ModTime(),
		Size:     info.Size(),
		IsLoaded: false,
	}

	s.fileNodes[path] = rootNode

	// Запускаем watching
	s.startWatching()

	// Обновляем UI
	if s.fileTree != nil {
		s.fileTree.Refresh()
	}
	s.updateStatus(fmt.Sprintf("Loaded: %s", path))

	if s.onPathChanged != nil {
		s.onPathChanged(path)
	}

	return nil
}

// loadDirectoryChildren загружает дочерние элементы директории
func (s *SidebarWidget) loadDirectoryChildren(node *FileNode) {
	if !node.IsDir || node.IsLoaded {
		return
	}

	entries, err := ioutil.ReadDir(node.Path)
	if err != nil {
		s.updateStatus(fmt.Sprintf("Error reading directory: %v", err))
		return
	}

	node.Children = []*FileNode{}

	for _, entry := range entries {
		// Пропускаем скрытые файлы если настроено
		if !s.showHiddenFiles && s.isHiddenFile(entry.Name()) {
			continue
		}

		childPath := filepath.Join(node.Path, entry.Name())

		childNode := &FileNode{
			Path:        childPath,
			Name:        entry.Name(),
			IsDir:       entry.IsDir(),
			Size:        entry.Size(),
			Modified:    entry.ModTime(),
			Extension:   filepath.Ext(entry.Name()),
			Parent:      node,
			Permissions: entry.Mode(),
			IsHidden:    s.isHiddenFile(entry.Name()),
			IsLoaded:    false,
		}

		node.Children = append(node.Children, childNode)
		s.fileNodes[childPath] = childNode
	}

	node.IsLoaded = true

	// Добавляем в watching
	s.addToWatching(node.Path)
}

// Методы поиска и фильтрации

// applyFilterAndSearch применяет текущие фильтры и поиск
func (s *SidebarWidget) applyFilterAndSearch() {
	s.loadMutex.Lock()
	defer s.loadMutex.Unlock()

	s.filteredNodes = make(map[string]*FileNode)

	for path, node := range s.fileNodes {
		if s.shouldShowNode(node) {
			s.filteredNodes[path] = node
		}
	}

	if s.fileTree != nil {
		// Обновление интерфейса должно выполняться в главном UI потоке
		fyne.Do(func() {
			s.fileTree.Refresh()
		})
	}

	s.updateSearchResults()
}

// shouldShowNode определяет, должен ли узел отображаться
func (s *SidebarWidget) shouldShowNode(node *FileNode) bool {
	// Проверяем скрытые файлы
	if !s.showHiddenFiles && node.IsHidden {
		return false
	}

	// Применяем фильтр по типу файла
	if !s.matchesFilter(node) {
		return false
	}

	// Применяем поиск
	if s.searchTerm != "" && !s.matchesSearch(node, s.searchTerm) {
		return false
	}

	return true
}

// matchesFilter проверяет соответствие узла текущему фильтру
func (s *SidebarWidget) matchesFilter(node *FileNode) bool {
	if s.activeFilter == "All" {
		return true
	}

	filter, exists := FileFilters[s.activeFilter]
	if !exists {
		return true
	}

	// Директории всегда показываем
	if node.IsDir {
		return true
	}

	// Проверяем расширение файла
	return filter.Pattern.MatchString(node.Name)
}

// matchesSearch проверяет соответствие узла поисковому запросу
func (s *SidebarWidget) matchesSearch(node *FileNode, searchTerm string) bool {
	if searchTerm == "" {
		return true
	}

	searchTerm = strings.ToLower(searchTerm)

	// Fuzzy search в имени файла
	fileName := strings.ToLower(node.Name)

	// Простой fuzzy match - проверяем вхождение всех символов поиска
	if s.fuzzyMatch(fileName, searchTerm) {
		return true
	}

	// Поиск по частичному совпадению
	return strings.Contains(fileName, searchTerm)
}

// fuzzyMatch выполняет fuzzy поиск
func (s *SidebarWidget) fuzzyMatch(text, pattern string) bool {
	if pattern == "" {
		return true
	}

	textRunes := []rune(text)
	patternRunes := []rune(pattern)

	textIndex := 0
	patternIndex := 0

	for textIndex < len(textRunes) && patternIndex < len(patternRunes) {
		if textRunes[textIndex] == patternRunes[patternIndex] {
			patternIndex++
		}
		textIndex++
	}

	return patternIndex == len(patternRunes)
}

// updateSearchResults обновляет результаты поиска
func (s *SidebarWidget) updateSearchResults() {
	if s.searchTerm == "" {
		s.searchResults = []string{}
		s.updateStatus(fmt.Sprintf("Files: %d", len(s.filteredNodes)))
		return
	}

	s.searchResults = []string{}
	for path, node := range s.filteredNodes {
		if !node.IsDir && s.matchesSearch(node, s.searchTerm) {
			s.searchResults = append(s.searchResults, path)
		}
	}

	s.updateStatus(fmt.Sprintf("Found %d files matching '%s'", len(s.searchResults), s.searchTerm))
}

// Методы сортировки

// sortPaths сортирует массив путей
func (s *SidebarWidget) sortPaths(paths []string) {
	sort.Slice(paths, func(i, j int) bool {
		nodeI := s.getNodeByPath(paths[i])
		nodeJ := s.getNodeByPath(paths[j])

		if nodeI == nil || nodeJ == nil {
			return false
		}

		// Директории всегда сверху
		if nodeI.IsDir != nodeJ.IsDir {
			return nodeI.IsDir
		}

		var result bool

		switch s.sortBy {
		case SortByName:
			result = strings.ToLower(nodeI.Name) < strings.ToLower(nodeJ.Name)
		case SortByType:
			result = nodeI.Extension < nodeJ.Extension
		case SortBySize:
			result = nodeI.Size < nodeJ.Size
		case SortByDate:
			result = nodeI.Modified.Before(nodeJ.Modified)
		default:
			result = strings.ToLower(nodeI.Name) < strings.ToLower(nodeJ.Name)
		}

		if !s.sortAscending {
			result = !result
		}

		return result
	})
}

// File watching methods

// startWatching запускает наблюдение за файловой системой
func (s *SidebarWidget) startWatching() {
	if s.watcher != nil {
		s.stopWatching()
	}

	var err error
	s.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		s.updateStatus(fmt.Sprintf("File watcher error: %v", err))
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.watcherCancel = cancel

	go s.watcherWorker(ctx)
}

// stopWatching останавливает наблюдение
func (s *SidebarWidget) stopWatching() {
	if s.watcherCancel != nil {
		s.watcherCancel()
	}

	if s.watcher != nil {
		s.watcher.Close()
		s.watcher = nil
	}

	s.watchedDirs = make(map[string]bool)
}

// addToWatching добавляет директорию в наблюдение
func (s *SidebarWidget) addToWatching(path string) {
	if s.watcher == nil {
		return
	}

	if s.watchedDirs[path] {
		return
	}

	err := s.watcher.Add(path)
	if err != nil {
		s.updateStatus(fmt.Sprintf("Cannot watch directory: %v", err))
		return
	}

	s.watchedDirs[path] = true
}

// watcherWorker обрабатывает события файловой системы
func (s *SidebarWidget) watcherWorker(ctx context.Context) {
	if s.watcher == nil {
		return
	}

	for {
		select {
		case event, ok := <-s.watcher.Events:
			if !ok {
				return
			}

			s.handleFileSystemEvent(event)

		case err, ok := <-s.watcher.Errors:
			if !ok {
				return
			}

			s.updateStatus(fmt.Sprintf("File watcher error: %v", err))

		case <-ctx.Done():
			return
		}
	}
}

// handleFileSystemEvent обрабатывает событие файловой системы
func (s *SidebarWidget) handleFileSystemEvent(event fsnotify.Event) {
	// Используем канал для debouncing множественных событий
	select {
	case s.refreshChan <- filepath.Dir(event.Name):
	default:
		// Канал заполнен, пропускаем
	}
}

// startRefreshWorker запускает воркер обновления
func (s *SidebarWidget) startRefreshWorker() {
	go func() {
		refreshMap := make(map[string]time.Time)

		for {
			select {
			case path := <-s.refreshChan:
				refreshMap[path] = time.Now()

			case <-time.After(500 * time.Millisecond):
				// Обрабатываем накопленные изменения
				now := time.Now()
				for path, timestamp := range refreshMap {
					if now.Sub(timestamp) > 300*time.Millisecond {
						s.refreshDirectory(path)
						delete(refreshMap, path)
					}
				}
			}
		}
	}()
}

// refreshDirectory обновляет директорию
func (s *SidebarWidget) refreshDirectory(path string) {
	s.loadMutex.Lock()

	node := s.getNodeByPath(path)
	if node == nil || !node.IsDir {
		s.loadMutex.Unlock()
		return
	}

	// Перезагружаем дочерние узлы
	node.IsLoaded = false
	s.loadDirectoryChildren(node)
	s.loadMutex.Unlock()

	// Обновляем отображение
	s.applyFilterAndSearch()
}

// Navigation methods

// NavigateUp переходит в родительскую директорию
func (s *SidebarWidget) NavigateUp() {
	if s.currentPath == s.rootPath {
		return
	}

	parentPath := filepath.Dir(s.currentPath)
	if parentPath != s.currentPath {
		s.SetRootPath(parentPath)
	}
}

// NavigateToHome переходит в домашнюю директорию
func (s *SidebarWidget) NavigateToHome() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		s.updateStatus(fmt.Sprintf("Cannot get home directory: %v", err))
		return
	}

	s.SetRootPath(homeDir)
}

// NavigateToPath переходит к указанному пути
func (s *SidebarWidget) NavigateToPath(path string) error {
	return s.SetRootPath(path)
}

// RefreshPath обновляет указанный путь
func (s *SidebarWidget) RefreshPath(path string) {
	if path == "" {
		path = s.currentPath
	}

	s.refreshDirectory(path)
	s.updateStatus("Refreshed")
}

// Context menu and file operations

// ShowContextMenu показывает контекстное меню для файла
func (s *SidebarWidget) ShowContextMenu(path string, pos fyne.Position) {
	node := s.getNodeByPath(path)
	if node == nil {
		return
	}

	menu := fyne.NewMenu("File Operations")

	// Открыть файл
	if !node.IsDir {
		menu.Items = append(menu.Items, fyne.NewMenuItem("Open", func() {
			if s.onFileOpened != nil {
				s.onFileOpened(path)
			}
		}))
		menu.Items = append(menu.Items, fyne.NewMenuItemSeparator())
	}

	// Открыть в проводнике
	menu.Items = append(menu.Items, fyne.NewMenuItem("Show in Explorer", func() {
		s.showInExplorer(path)
	}))

	menu.Items = append(menu.Items, fyne.NewMenuItemSeparator())

	// Операции с файлами
	menu.Items = append(menu.Items, fyne.NewMenuItem("Copy Path", func() {
		s.copyPathToClipboard(path)
	}))

	menu.Items = append(menu.Items, fyne.NewMenuItem("Rename", func() {
		s.showRenameDialog(node)
	}))

	menu.Items = append(menu.Items, fyne.NewMenuItem("Delete", func() {
		s.showDeleteDialog(node)
	}))

	menu.Items = append(menu.Items, fyne.NewMenuItemSeparator())

	// Создание новых файлов/папок
	if node.IsDir {
		menu.Items = append(menu.Items, fyne.NewMenuItem("New File", func() {
			s.showNewFileDialog(node)
		}))

		menu.Items = append(menu.Items, fyne.NewMenuItem("New Folder", func() {
			s.showNewFolderDialog(node)
		}))
	}

	// Свойства файла
	menu.Items = append(menu.Items, fyne.NewMenuItemSeparator())
	menu.Items = append(menu.Items, fyne.NewMenuItem("Properties", func() {
		s.showPropertiesDialog(node)
	}))

	// Показываем меню (в Fyne нет popup menu, используем диалог)
	s.showContextMenuDialog(menu)
}

// File operation implementations

// showInExplorer открывает файл в проводнике Windows
func (s *SidebarWidget) showInExplorer(path string) {
	cmd := fmt.Sprintf(`explorer /select,"%s"`, path)
	err := exec.Command("cmd", "/c", cmd).Start()
	if err != nil {
		s.updateStatus(fmt.Sprintf("Cannot open explorer: %v", err))
	}
}

// copyPathToClipboard копирует путь в буфер обмена
func (s *SidebarWidget) copyPathToClipboard(path string) {
	clipboard := fyne.CurrentApp().Driver().AllWindows()[0].Clipboard()
	clipboard.SetContent(path)
	s.updateStatus("Path copied to clipboard")
}

// showRenameDialog показывает диалог переименования
func (s *SidebarWidget) showRenameDialog(node *FileNode) {
	entry := widget.NewEntry()
	entry.SetText(node.Name)

	dialog.ShowForm("Rename", "Rename", "Cancel", []*widget.FormItem{
		{Text: "New name:", Widget: entry},
	}, func(confirmed bool) {
		if !confirmed || entry.Text == "" || entry.Text == node.Name {
			return
		}

		s.renameFile(node, entry.Text)
	}, fyne.CurrentApp().Driver().AllWindows()[0])
}

// showDeleteDialog показывает диалог удаления
func (s *SidebarWidget) showDeleteDialog(node *FileNode) {
	fileType := "file"
	if node.IsDir {
		fileType = "folder"
	}

	message := fmt.Sprintf("Are you sure you want to delete the %s '%s'?", fileType, node.Name)

	dialog.ShowConfirm("Delete", message, func(confirmed bool) {
		if confirmed {
			s.deleteFile(node)
		}
	}, fyne.CurrentApp().Driver().AllWindows()[0])
}

// showNewFileDialog показывает диалог создания файла
func (s *SidebarWidget) showNewFileDialog(parentNode *FileNode) {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("filename.ext")

	dialog.ShowForm("New File", "Create", "Cancel", []*widget.FormItem{
		{Text: "File name:", Widget: entry},
	}, func(confirmed bool) {
		if !confirmed || entry.Text == "" {
			return
		}

		s.createNewFile(parentNode, entry.Text)
	}, fyne.CurrentApp().Driver().AllWindows()[0])
}

// showNewFolderDialog показывает диалог создания папки
func (s *SidebarWidget) showNewFolderDialog(parentNode *FileNode) {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("New Folder")

	dialog.ShowForm("New Folder", "Create", "Cancel", []*widget.FormItem{
		{Text: "Folder name:", Widget: entry},
	}, func(confirmed bool) {
		if !confirmed || entry.Text == "" {
			return
		}

		s.createNewFolder(parentNode, entry.Text)
	}, fyne.CurrentApp().Driver().AllWindows()[0])
}

// showPropertiesDialog показывает диалог свойств файла
func (s *SidebarWidget) showPropertiesDialog(node *FileNode) {
	info := []string{
		fmt.Sprintf("Name: %s", node.Name),
		fmt.Sprintf("Path: %s", node.Path),
		fmt.Sprintf("Type: %s", s.getFileType(node)),
		fmt.Sprintf("Size: %s", s.formatFileSize(node.Size)),
		fmt.Sprintf("Modified: %s", node.Modified.Format("2006-01-02 15:04:05")),
		fmt.Sprintf("Permissions: %s", node.Permissions.String()),
	}

	if node.Extension != "" {
		info = append(info, fmt.Sprintf("Extension: %s", node.Extension))
	}

	content := widget.NewLabel(strings.Join(info, "\n"))
	content.Wrapping = fyne.TextWrapWord

	dialog.ShowCustom("Properties", "Close", content, fyne.CurrentApp().Driver().AllWindows()[0])
}

// File operation implementations

// renameFile переименовывает файл
func (s *SidebarWidget) renameFile(node *FileNode, newName string) {
	oldPath := node.Path
	newPath := filepath.Join(filepath.Dir(oldPath), newName)

	err := os.Rename(oldPath, newPath)
	if err != nil {
		dialog.ShowError(fmt.Errorf("cannot rename file: %v", err), fyne.CurrentApp().Driver().AllWindows()[0])
		return
	}

	// Обновляем узел
	node.Path = newPath
	node.Name = newName
	node.Extension = filepath.Ext(newName)

	// Обновляем карту
	delete(s.fileNodes, oldPath)
	s.fileNodes[newPath] = node

	s.fileTree.Refresh()
	s.updateStatus(fmt.Sprintf("Renamed to: %s", newName))
}

// deleteFile удаляет файл
func (s *SidebarWidget) deleteFile(node *FileNode) {
	err := os.RemoveAll(node.Path)
	if err != nil {
		dialog.ShowError(fmt.Errorf("cannot delete file: %v", err), fyne.CurrentApp().Driver().AllWindows()[0])
		return
	}

	// Удаляем из parent
	if node.Parent != nil {
		for i, child := range node.Parent.Children {
			if child.Path == node.Path {
				node.Parent.Children = append(node.Parent.Children[:i], node.Parent.Children[i+1:]...)
				break
			}
		}
	}

	// Удаляем из карты
	delete(s.fileNodes, node.Path)

	s.fileTree.Refresh()
	s.updateStatus(fmt.Sprintf("Deleted: %s", node.Name))
}

// createNewFile создает новый файл
func (s *SidebarWidget) createNewFile(parentNode *FileNode, fileName string) {
	newPath := filepath.Join(parentNode.Path, fileName)

	// Проверяем существование
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		dialog.ShowError(fmt.Errorf("file already exists: %s", fileName), fyne.CurrentApp().Driver().AllWindows()[0])
		return
	}

	// Создаем файл
	file, err := os.Create(newPath)
	if err != nil {
		dialog.ShowError(fmt.Errorf("cannot create file: %v", err), fyne.CurrentApp().Driver().AllWindows()[0])
		return
	}
	file.Close()

	// Обновляем дерево
	s.refreshDirectory(parentNode.Path)
	s.updateStatus(fmt.Sprintf("Created file: %s", fileName))
}

// createNewFolder создает новую папку
func (s *SidebarWidget) createNewFolder(parentNode *FileNode, folderName string) {
	newPath := filepath.Join(parentNode.Path, folderName)

	err := os.Mkdir(newPath, 0755)
	if err != nil {
		dialog.ShowError(fmt.Errorf("cannot create folder: %v", err), fyne.CurrentApp().Driver().AllWindows()[0])
		return
	}

	// Обновляем дерево
	s.refreshDirectory(parentNode.Path)
	s.updateStatus(fmt.Sprintf("Created folder: %s", folderName))
}

// Settings and preferences

// showSettingsDialog показывает диалог настроек
func (s *SidebarWidget) showSettingsDialog() {
	// Показать скрытые файлы
	hiddenCheck := widget.NewCheck("Show hidden files", func(checked bool) {
		s.showHiddenFiles = checked
		s.applyFilterAndSearch()
	})
	hiddenCheck.SetChecked(s.showHiddenFiles)

	// Сортировка
	sortOptions := []string{"Name", "Type", "Size", "Date"}
	sortSelect := widget.NewSelect(sortOptions, func(selected string) {
		switch selected {
		case "Name":
			s.sortBy = SortByName
		case "Type":
			s.sortBy = SortByType
		case "Size":
			s.sortBy = SortBySize
		case "Date":
			s.sortBy = SortByDate
		}
		s.applyFilterAndSearch()
	})
	sortSelect.SetSelected(sortOptions[int(s.sortBy)])

	// Порядок сортировки
	ascendingCheck := widget.NewCheck("Ascending", func(checked bool) {
		s.sortAscending = checked
		s.applyFilterAndSearch()
	})
	ascendingCheck.SetChecked(s.sortAscending)

	form := dialog.NewForm("Sidebar Settings", "Apply", "Cancel", []*widget.FormItem{
		{Text: "Display:", Widget: hiddenCheck},
		{Text: "Sort by:", Widget: sortSelect},
		{Text: "Order:", Widget: ascendingCheck},
	}, func(confirmed bool) {
		if confirmed {
			s.updateStatus("Settings applied")
		}
	}, fyne.CurrentApp().Driver().AllWindows()[0])

	form.Show()
}

// Utility methods

// getNodeByPath возвращает узел по пути
func (s *SidebarWidget) getNodeByPath(path string) *FileNode {
	if node, exists := s.fileNodes[path]; exists {
		return node
	}
	return nil
}

// getNodeIcon возвращает иконку для узла
func (s *SidebarWidget) getNodeIcon(node *FileNode) fyne.Resource {
	if node.IsDir {
		return theme.FolderIcon()
	}

	// Определяем иконку по расширению
	switch strings.ToLower(node.Extension) {
	case ".go":
		return theme.DocumentIcon() // В будущем можно добавить специальные иконки
	case ".py":
		return theme.DocumentIcon()
	case ".rs":
		return theme.DocumentIcon()
	case ".c", ".h", ".cpp", ".hpp":
		return theme.DocumentIcon()
	case ".java":
		return theme.DocumentIcon()
	case ".js", ".ts":
		return theme.DocumentIcon()
	case ".txt", ".md":
		return theme.DocumentIcon()
	case ".json", ".xml", ".yaml", ".yml":
		return theme.DocumentIcon()
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp":
		return theme.DocumentIcon()
	case ".mp3", ".wav", ".ogg":
		return theme.DocumentIcon()
	case ".mp4", ".avi", ".mkv":
		return theme.DocumentIcon()
	default:
		return theme.DocumentIcon()
	}
}

// formatFileSize форматирует размер файла
func (s *SidebarWidget) formatFileSize(size int64) string {
	if size == 0 {
		return "0 B"
	}

	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}

	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	units := []string{"KB", "MB", "GB", "TB"}
	return fmt.Sprintf("%.1f %s", float64(size)/float64(div), units[exp])
}

// getFileType возвращает тип файла
func (s *SidebarWidget) getFileType(node *FileNode) string {
	if node.IsDir {
		return "Folder"
	}

	if node.Extension == "" {
		return "File"
	}

	return fmt.Sprintf("%s file", strings.ToUpper(node.Extension[1:]))
}

// isHiddenFile проверяет, является ли файл скрытым
func (s *SidebarWidget) isHiddenFile(name string) bool {
	// В Windows скрытые файлы обычно начинаются с точки (Unix convention)
	// или имеют атрибут Hidden (но это сложнее проверить)
	return strings.HasPrefix(name, ".")
}

// updateStatus обновляет статусную строку
func (s *SidebarWidget) updateStatus(message string) {
	if s.statusLabel != nil {
		fyne.Do(func() {
			s.statusLabel.SetText(message)
		})
	}
}

// showContextMenuDialog показывает контекстное меню как диалог
func (s *SidebarWidget) showContextMenuDialog(menu *fyne.Menu) {
	buttons := []fyne.CanvasObject{}

	for _, item := range menu.Items {
		if item.Label == "-" { // в fyne разделитель указывают через Label = "-"
			buttons = append(buttons, widget.NewSeparator())
		} else {
			btn := widget.NewButton(item.Label, item.Action)
			buttons = append(buttons, btn)
		}
	}

	content := container.NewVBox(buttons...)
	dialog.NewCustom(
		"File Operations",
		"Close",
		content,
		fyne.CurrentApp().Driver().AllWindows()[0],
	).Show()
}

// Public API methods

// SetVisible устанавливает видимость сайдбара
func (s *SidebarWidget) SetVisible(visible bool) {
	s.isVisible = visible
	if s.mainContainer != nil {
		if visible {
			AnimateShow(s.mainContainer)
		} else {
			AnimateHide(s.mainContainer)
		}
	}
}

// IsVisible возвращает состояние видимости
func (s *SidebarWidget) IsVisible() bool {
	return s.isVisible
}

// GetSelectedFile возвращает выбранный файл
func (s *SidebarWidget) GetSelectedFile() string {
	return s.selectedFile
}

// GetCurrentPath возвращает текущий путь
func (s *SidebarWidget) GetCurrentPath() string {
	return s.currentPath
}

// SetCallbacks устанавливает callback функции
func (s *SidebarWidget) SetCallbacks(onFileSelected, onFileOpened func(string), onPathChanged func(string)) {
	s.onFileSelected = onFileSelected
	s.onFileOpened = onFileOpened
	s.onPathChanged = onPathChanged
}

// CreateRenderer реализует интерфейс fyne.Widget
func (s *SidebarWidget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(s.mainContainer)
}

// Cleanup очищает ресурсы при закрытии
func (s *SidebarWidget) Cleanup() {
	s.stopWatching()
}
