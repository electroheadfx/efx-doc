package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/paginator"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"gopkg.in/yaml.v3"
)

// Create goldmark with table extension for web preview
var webMarkdown = goldmark.New(
	goldmark.WithExtensions(extension.Table),
)

// isTerminal checks if we're running in a terminal
func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// Version info
const AppName = "efx-doc"
const Version = "0.1.0"

// Key bindings
type keyMap struct {
	up              key.Binding
	down            key.Binding
	left            key.Binding
	right           key.Binding
	enter           key.Binding
	space           key.Binding
	tab             key.Binding
	pgup            key.Binding
	pgdown          key.Binding
	quit            key.Binding
	search          key.Binding
	switchWorkspace key.Binding
}

var keys = keyMap{
	up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "move up"),
	),
	down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "move down"),
	),
	left: key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("←/h", "left panel"),
	),
	right: key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("→/l", "right panel"),
	),
	enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("Enter", "preview"),
	),
	space: key.NewBinding(
		key.WithKeys("space"),
		key.WithHelp("Space", "preview"),
	),
	tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("Tab", "category"),
	),
	pgup: key.NewBinding(
		key.WithKeys("pgup"),
		key.WithHelp("PgUp", "page up"),
	),
	pgdown: key.NewBinding(
		key.WithKeys("pgdown"),
		key.WithHelp("PgDn", "page down"),
	),
	quit: key.NewBinding(
		key.WithKeys("ctrl+c", "q"),
		key.WithHelp("q", "quit"),
	),
	search: key.NewBinding(
		key.WithKeys("/", "?"),
		key.WithHelp("/", "search"),
	),
	switchWorkspace: key.NewBinding(
		key.WithKeys("W"),
		key.WithHelp("W", "switch workspace"),
	),
}

// Config represents the documentation structure
type Config struct {
	Name        string     `yaml:"name"`
	Description string     `yaml:"description"`
	Categories  []Category `yaml:"categories"`
}

type Category struct {
	Name       string      `yaml:"name"`
	References []Reference `yaml:"references"`
}

type Reference struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// WorkspaceConfig represents the workspace configuration
type WorkspaceConfig struct {
	Workspaces []Workspace `yaml:"workspaces"`
}

type Workspace struct {
	Name   string `yaml:"name"`
	Path   string `yaml:"path"`
	Styles Styles `yaml:"styles"`
}

type Styles struct {
	TUI string `yaml:"tui"`
	Web string `yaml:"web"`
}

// Global workspace variables
var (
	currentWorkspace *Workspace
	configDir        string
)

// item implements list.Item interface
type item struct {
	name        string
	description string
	category    string
}

func (i item) Title() string       { return i.name }
func (i item) Description() string { return i.description }
func (i item) FilterValue() string { return i.name + " " + i.description + " " + i.category }

// Global glamour renderer - created once, reused
var glamourRenderer *glamour.TermRenderer
var glamourRenderFunc func(string) (string, error)

// Global HTTP server control
var (
	httpServer     *http.Server
	currentHTML    string
	currentConfig  *Config // Store config for web navigation
	currentDocName string  // Current document name
	currentCatName string  // Current category name
)

// Get the config directory path
func getConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.Getenv("HOME"), ".config", "efx-doc")
	}
	return filepath.Join(home, ".config", "efx-doc")
}

// ExpandTilde expands ~ to the user's home directory
func ExpandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// LoadWorkspaceConfig loads the workspace configuration
func LoadWorkspaceConfig() (*WorkspaceConfig, error) {
	configPath := filepath.Join(getConfigDir(), "workspaces.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var config WorkspaceConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// Workspace item for list
type workspaceItem struct {
	workspace Workspace
}

func (w *workspaceItem) Title() string       { return w.workspace.Name }
func (w *workspaceItem) Description() string { return w.workspace.Path }
func (w *workspaceItem) FilterValue() string { return w.workspace.Name }

// Workspace selector model
type workspaceSelector struct {
	list       list.Model
	workspaces []Workspace
	selected   *Workspace // Store selected workspace
}

func newWorkspaceSelector(workspaces []Workspace) workspaceSelector {
	items := make([]list.Item, len(workspaces))
	for i, ws := range workspaces {
		items[i] = &workspaceItem{workspace: ws}
	}

	// Use default delegate with larger size to fit all items
	l := list.New(items, list.NewDefaultDelegate(), 60, 20)
	l.Title = "Select a workspace"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetSize(60, 20)

	return workspaceSelector{
		list:       l,
		workspaces: workspaces,
	}
}

func (m workspaceSelector) Init() tea.Cmd {
	return nil
}

func (m workspaceSelector) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			if idx := m.list.Index(); idx >= 0 && idx < len(m.workspaces) {
				m.selected = &m.workspaces[idx]
				return m, tea.Quit
			}
		case tea.KeyTab:
			// Tab moves to next workspace
			currentIdx := m.list.Index()
			m.list.CursorUp()
			if m.list.Index() == currentIdx {
				// Already at top, wrap to bottom
				for i := 0; i < len(m.workspaces)-1; i++ {
					m.list.CursorDown()
				}
			}
		case tea.KeyShiftTab:
			// Shift+Tab moves to previous workspace
			m.list.CursorDown()
		case tea.KeyEsc, tea.KeyCtrlC:
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m workspaceSelector) View() string {
	return "\n" + m.list.View()
}

// the Bubble Tea workspace RunWorkspaceSelector runs selector
func RunWorkspaceSelector(workspaces []Workspace) *Workspace {
	if len(workspaces) == 0 {
		return nil
	}

	// If only one workspace, use it directly
	if len(workspaces) == 1 {
		fmt.Println("Using workspace: " + workspaces[0].Name)
		return &workspaces[0]
	}

	model := newWorkspaceSelector(workspaces)
	p := tea.NewProgram(model, tea.WithAltScreen())

	result, err := p.Run()
	if err != nil {
		fmt.Println("Error running selector:", err)
		return &workspaces[0]
	}

	// Access selected workspace from the model
	if finalModel, ok := result.(workspaceSelector); ok && finalModel.selected != nil {
		return finalModel.selected
	}

	return &workspaces[0]
}

// SelectWorkspace prompts the user to select a workspace using Bubble Tea
func SelectWorkspace(config *WorkspaceConfig) *Workspace {
	if config == nil || len(config.Workspaces) == 0 {
		return nil
	}

	// Use Bubble Tea selector
	return RunWorkspaceSelector(config.Workspaces)
}

// GetWorkspaceDocsPath returns the docs path for the current workspace
func GetWorkspaceDocsPath() string {
	if currentWorkspace == nil {
		return ""
	}
	return ExpandTilde(currentWorkspace.Path)
}

// GetStylePath returns the path to the TUI style file
func GetStylePath() string {
	if currentWorkspace == nil {
		return filepath.Join(getConfigDir(), "opencode_style.json")
	}
	return ExpandTilde(currentWorkspace.Styles.TUI)
}

func initGlamour(width int) {
	// Try to load opencode style from file
	stylePath := GetStylePath()
	styleContent, err := os.ReadFile(stylePath)

	var rendererOption glamour.TermRendererOption
	if err == nil {
		rendererOption = glamour.WithStylesFromJSONBytes(styleContent)
	} else {
		// Fallback to standard dracula style
		rendererOption = glamour.WithStandardStyle("dracula")
	}

	// Create renderer
	glamourRenderer, err = glamour.NewTermRenderer(
		rendererOption,
		glamour.WithWordWrap(width),
	)

	if err != nil {
		fmt.Println("Error creating renderer:", err)
		// Fallback if custom style fails (e.g. invalid JSON)
		glamourRenderer, _ = glamour.NewTermRenderer(
			glamour.WithStandardStyle("dracula"),
			glamour.WithWordWrap(width),
		)
	}

	// Function wrapper
	glamourRenderFunc = func(content string) (string, error) {
		if glamourRenderer != nil {
			return glamourRenderer.Render(content)
		}
		return "", fmt.Errorf("renderer not initialized")
	}
}

// RenderMarkdown renders markdown content using glamour
func RenderMarkdown(content string, width int) string {
	// Try with Render function first (better styling)
	if glamourRenderFunc != nil {
		if result, err := glamourRenderFunc(content); err == nil && result != "" {
			return result
		}
	}
	// Fallback to renderer
	if glamourRenderer != nil {
		if result, err := glamourRenderer.Render(content); err == nil && result != "" {
			return result
		}
	}
	return content
}

// Get the docs directory from workspace config
func getDataDir() string {
	return GetWorkspaceDocsPath()
}

// Styles
var (
	// Left panel styles
	activeTabStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 2).
			Bold(true)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#E0B7EE")).
				Padding(0, 2)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4")).
			Bold(true)

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FAFAFA"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			MarginTop(1)

	// Right panel (doc preview) styles
	docTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4")).
			MarginBottom(1)

	docCodeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575")).
			Background(lipgloss.Color("#1a1a1a")).
			Padding(0, 1)

	docBorderStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), true).
			BorderForeground(lipgloss.Color("#333333"))

	focusedBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder(), true).
				BorderForeground(lipgloss.Color("#7D56F4"))

	unfocusedBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder(), true).
				BorderForeground(lipgloss.Color("#222222"))
)

type model struct {
	config        Config
	items         []item
	filteredItems []item
	cursor        int
	currentPage   int
	itemsPerPage  int
	filter        string
	filtering     bool
	quitting      bool
	selected      *item
	width         int
	height        int
	activeTab     int
	docContent    string
	viewport      viewport.Model
	paginator     paginator.Model
	focusRight    bool              // true = right panel, false = left panel
	docCache      map[string]string // Cache rendered markdown by doc name
	docCacheKey   string            // Current cached doc key
	docPath       string            // Current doc file path
	toast         string            // Toast message to display
	toastTimer    int               // Timer for toast auto-hide
	serverRunning bool              // Is HTTP server running
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.itemsPerPage = (m.height - 12) / 2
		if m.itemsPerPage < 5 {
			m.itemsPerPage = 5
		}

		// Update viewport size
		// Left panel uses 40% of width
		leftWidth := m.width * 40 / 100
		if leftWidth < 45 {
			leftWidth = 45
		}

		// Doc panel uses remaining width
		docWidth := m.width - leftWidth - 4
		if docWidth < 40 {
			docWidth = 40
		}

		docHeight := m.height - 4
		if docHeight < 10 {
			docHeight = 10
		}
		m.viewport = viewport.New(docWidth, docHeight)

		// Re-init glamour with new width
		initGlamour(docWidth)

		// Re-render current doc if any
		if m.docCacheKey != "" {
			m.loadDoc(m.docCacheKey)
		}

		return m, nil

	case tea.KeyMsg:
		if m.filtering {
			switch msg.String() {
			case "enter":
				m.filtering = false
				if len(m.filteredItems) > 0 {
					m.cursor = 0
				}
				return m, nil
			case "esc":
				m.filtering = false
				m.filter = ""
				m.filterByTab()
				return m, nil
			case "backspace":
				if len(m.filter) > 0 {
					m.filter = m.filter[:len(m.filter)-1]
					m.applyFilter()
				}
				return m, nil
			default:
				if len(msg.String()) == 1 {
					m.filter += msg.String()
					m.applyFilter()
				}
				return m, nil
			}
		}

		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		case "?":
			m.filtering = true
			m.activeTab = 0
			return m, nil
		case "/":
			m.filtering = true
			m.activeTab = 0
			return m, nil
		case "esc":
			if m.filter != "" {
				m.filter = ""
				m.filterByTab()
			}
			return m, nil
		case "up", "j":
			if len(m.filteredItems) > 0 {
				m.cursor = (m.cursor - 1 + len(m.filteredItems)) % len(m.filteredItems)
				m.currentPage = m.cursor / m.getItemsPerPage()
				m.loadDoc(m.filteredItems[m.cursor].name)
				// Auto-sync to web
				if m.serverRunning {
					catName := m.filteredItems[m.cursor].category
					updateWebPreview(m.filteredItems[m.cursor].name, catName, m.docContent)
				}
			}
		case "down", "k", " ":
			if len(m.filteredItems) > 0 {
				m.cursor = (m.cursor + 1) % len(m.filteredItems)
				m.currentPage = m.cursor / m.getItemsPerPage()
				m.loadDoc(m.filteredItems[m.cursor].name)
				// Auto-sync to web
				if m.serverRunning {
					catName := m.filteredItems[m.cursor].category
					updateWebPreview(m.filteredItems[m.cursor].name, catName, m.docContent)
				}
			}
		case "pgup":
			m.viewport.PageUp()
		case "pgdown":
			m.viewport.PageDown()
		case "right":
			m.viewport.LineDown(5)
		case "left":
			m.viewport.LineUp(5)
		case "tab":
			m.activeTab = (m.activeTab + 1) % (len(m.config.Categories) + 1)
			m.filter = ""
			m.filterByTab()
			m.cursor = 0
			m.currentPage = 0
		case "shift+tab":
			m.activeTab--
			if m.activeTab < 0 {
				m.activeTab = len(m.config.Categories)
			}
			m.filter = ""
			m.filterByTab()
			m.cursor = 0
			m.currentPage = 0
		case "enter":
			// Copy current doc content to clipboard
			if m.docContent != "" {
				clipboard.WriteAll(m.docContent)
				m.toast = "Copied!"
				m.toastTimer = 30 // Show for 30 ticks
			}
		case "f":
			// Open file location in Finder
			if m.docPath != "" {
				exec.Command("open", filepath.Dir(m.docPath)).Start()
			}
		case "w":
			// Open web preview
			if m.docContent != "" {
				if m.serverRunning {
					// Server already running, just open browser
					m.toast = "Opening..."
					m.toastTimer = 30
					go exec.Command("open", "http://localhost:8080").Start()
				} else {
					// Start server
					m.serverRunning = true
					m.toast = "Web preview on :8080"
					m.toastTimer = 30
					if m.docCacheKey != "welcome" {
						// Find category for current doc
						catName := ""
						for _, item := range m.items {
							if item.name == m.docCacheKey {
								catName = item.category
								break
							}
						}
						go serveMarkdown(m.docCacheKey, m.docContent, catName, m.docCacheKey)
					} else {
						welcomeContent := generateWelcomeContent(&m.config, getDataDir())
						go serveMarkdown("preview", welcomeContent, "Overview", "README")
					}
				}
			}
		case "s":
			// Stop web server
			if httpServer != nil {
				httpServer.Close()
				httpServer = nil
				m.serverRunning = false
				m.toast = "Server stopped"
				m.toastTimer = 30
			}
		case "W":
			// Switch workspace - stop server first and return to workspace selector
			if httpServer != nil {
				httpServer.Close()
				httpServer = nil
				m.serverRunning = false
			}
			// Load workspace config and show selector
			workspaceConfig, err := LoadWorkspaceConfig()
			if err != nil {
				m.toast = "Failed to load workspaces"
				m.toastTimer = 30
				return m, nil
			}
			selected := RunWorkspaceSelector(workspaceConfig.Workspaces)
			if selected != nil {
				currentWorkspace = selected
				// Reload config from new workspace
				dataDir := getDataDir()
				configPath := filepath.Join(dataDir, "docs.yaml")
				configData, err := os.ReadFile(configPath)
				if err != nil {
					m.toast = "Failed to load docs config"
					m.toastTimer = 30
					return m, nil
				}
				config, err := loadConfig(configData)
				if err != nil {
					m.toast = "Failed to parse docs config"
					m.toastTimer = 30
					return m, nil
				}
				m.config = *config
				m.items = createItems(config)
				m.filteredItems = m.items
				m.activeTab = 0
				m.cursor = 0
				m.currentPage = 0
				m.filter = ""
				// Generate welcome content for new workspace
				welcomeContent := generateWelcomeContent(config, dataDir)
				welcomeRendered := RenderMarkdown(welcomeContent, 60)
				m.viewport.SetContent(welcomeRendered)
				m.docContent = welcomeContent
				m.toast = "Switched to: " + selected.Name
				m.toastTimer = 30
			}
		}
	}

	// Handle toast timer
	if m.toastTimer > 0 {
		m.toastTimer--
		if m.toastTimer == 0 {
			m.toast = ""
		}
	}

	// Handle mouse scroll for viewport
	if msg, ok := msg.(tea.MouseMsg); ok {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.viewport.LineUp(3)
		case tea.MouseButtonWheelDown:
			m.viewport.LineDown(3)
		}
	}

	m.viewport, _ = m.viewport.Update(msg)
	return m, nil
}

func (m *model) loadDoc(name string) {
	if cached, ok := m.docCache[name]; ok {
		m.viewport.SetContent(cached)
		m.docCacheKey = name
		return
	}

	// Handle README specially - use welcome content
	if name == "README" {
		welcomeContent := generateWelcomeContent(&m.config, getDataDir())
		rendered := RenderMarkdown(welcomeContent, m.width*60/100)
		m.docCache[name] = rendered
		m.viewport.SetContent(rendered)
		m.docCacheKey = name
		m.docPath = filepath.Join(getDataDir(), "docs", "README.md")
		return
	}

	var category string
	for _, i := range m.items {
		if i.name == name {
			category = i.category
			break
		}
	}

	categoryMap := map[string]string{
		"Core":       "core",
		"Responsive": "responsive",
		"Helpers":    "helpers",
		"Components": "components",
		"Templates":  "templates",
		"Player":     "player",
	}

	folder := categoryMap[category]
	if folder == "" {
		folder = "core"
	}

	dataDir := getDataDir()
	docsDir := filepath.Join(dataDir, folder)

	possibleNames := []string{
		name + ".md",
		strings.ReplaceAll(name, " ", "-") + ".md",
		strings.ReplaceAll(name, " ", "") + ".md",
	}

	var content []byte
	var foundPath string
	for _, fileName := range possibleNames {
		fullPath := filepath.Join(docsDir, fileName)
		content, _ = os.ReadFile(fullPath)
		if content != nil {
			foundPath = fullPath
			break
		}
	}

	if content == nil {
		m.docContent = fmt.Sprintf("# %s\n\nDocumentation not found.\n\nCategory: '%s'\nFolder: '%s'", name, category, folder)
		m.docCache[name] = m.docContent
		m.viewport.SetContent(m.docContent)
		m.docCacheKey = name
		m.docPath = ""
		return
	}

	m.docContent = string(content)

	leftWidth := m.width * 40 / 100
	if leftWidth < 45 {
		leftWidth = 45
	}
	docWidth := m.width - leftWidth - 4

	rendered := RenderMarkdown(m.docContent, docWidth)
	if rendered == "" {
		rendered = m.docContent
	}

	m.docCache[name] = rendered
	m.viewport.SetContent(rendered)
	m.docCacheKey = name
	m.docPath = foundPath
}

func (m *model) applyFilter() {
	if m.filter == "" {
		m.filterByTab()
		return
	}

	filter := strings.ToLower(m.filter)

	// Global search: always search ALL items
	m.filteredItems = []item{}
	for _, i := range m.items {
		if strings.Contains(strings.ToLower(i.name), filter) ||
			strings.Contains(strings.ToLower(i.description), filter) ||
			strings.Contains(strings.ToLower(i.category), filter) {
			m.filteredItems = append(m.filteredItems, i)
		}
	}
	m.cursor = 0
	m.currentPage = 0
}

func (m *model) filterByTab() {
	if m.activeTab == 0 {
		m.filteredItems = m.items
		return
	}

	catName := m.config.Categories[m.activeTab-1].Name
	m.filteredItems = []item{}
	for _, i := range m.items {
		if i.category == catName {
			m.filteredItems = append(m.filteredItems, i)
		}
	}
}

func (m model) getItemsPerPage() int {
	perPage := m.height - 14
	if perPage < 5 {
		perPage = 10
	}
	return perPage
}

func (m model) getTotalPages() int {
	perPage := m.getItemsPerPage()
	if len(m.filteredItems) == 0 {
		return 1
	}
	pages := (len(m.filteredItems) + perPage - 1) / perPage
	return pages
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	// Layout calculation
	leftWidth := m.width * 40 / 100
	if leftWidth < 45 {
		leftWidth = 45
	}

	docWidth := m.width - leftWidth - 4
	if docWidth < 40 {
		docWidth = 40
	}

	// Left panel
	var left strings.Builder

	// Title - with version on right
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4"))
	versionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))

	title := fmt.Sprintf("%s  %s", titleStyle.Render("efx-motion Docs"), versionStyle.Render(AppName+" "+Version))
	left.WriteString(title)
	left.WriteString("\n\n")

	// Tab bar
	var tabsBuilder strings.Builder
	tabs := []string{"All"}
	for _, cat := range m.config.Categories {
		tabs = append(tabs, cat.Name)
	}

	for i, tab := range tabs {
		if i == m.activeTab {
			tabsBuilder.WriteString(activeTabStyle.Render(tab))
		} else {
			tabsBuilder.WriteString(inactiveTabStyle.Render(tab))
		}
	}
	left.WriteString(tabsBuilder.String())
	left.WriteString("\n\n")

	// Filter input
	if m.filtering {
		left.WriteString("/ " + m.filter + "_")
		left.WriteString("\n\n")
	}

	// Column widths - dynamic based on left panel width
	// leftWidth available space: leftWidth - 4 (padding/border)
	availableWidth := leftWidth - 6
	nameWidth := 25
	if nameWidth > availableWidth/2 {
		nameWidth = availableWidth / 2
	}
	descWidth := availableWidth - nameWidth - 2

	headerStyle := dimStyle.Copy().Bold(true)

	header := fmt.Sprintf("  %-*s  %s", nameWidth, "Reference", "Description")
	left.WriteString(headerStyle.Render(header))
	left.WriteString("\n")
	left.WriteString(dimStyle.Render("  " + strings.Repeat("─", availableWidth+2)))
	left.WriteString("\n")

	// Items - Page based rendering
	visibleItems := m.filteredItems
	perPage := m.getItemsPerPage()
	totalPages := m.getTotalPages()

	// Ensure current page is valid
	if m.currentPage >= totalPages && totalPages > 0 {
		m.currentPage = totalPages - 1
	}
	if m.currentPage < 0 {
		m.currentPage = 0
	}

	startIdx := m.currentPage * perPage
	endIdx := startIdx + perPage
	if endIdx > len(visibleItems) {
		endIdx = len(visibleItems)
	}
	if startIdx > len(visibleItems) {
		startIdx = len(visibleItems)
	}

	renderedLines := 0
	for idx := startIdx; idx < endIdx; idx++ {
		i := visibleItems[idx]

		isSelected := idx == m.cursor
		prefix := "  "
		nameStyle := normalStyle
		descStyle := dimStyle

		if isSelected {
			prefix = "▸ "
			nameStyle = selectedStyle
			descStyle = dimStyle.Copy().Foreground(lipgloss.Color("#888888"))
		}

		nameCol := nameStyle.Render(fmt.Sprintf("%s%-*s", prefix, nameWidth, truncate(i.name, nameWidth)))
		descCol := descStyle.Render(truncate(i.description, descWidth))
		left.WriteString(fmt.Sprintf("%s  %s\n", nameCol, descCol))
		renderedLines++
	}

	// Pad with empty lines
	for i := renderedLines; i < perPage; i++ {
		left.WriteString("\n")
	}

	// Empty state
	if len(visibleItems) == 0 {
		left.WriteString(dimStyle.Render("  No references found\n"))
	}

	// Bullet pagination for left panel (like col 2)
	if totalPages > 1 {
		var pagination strings.Builder
		pagination.WriteString("  ")

		activeBullet := lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Bold(true)
		inactiveBullet := lipgloss.NewStyle().Foreground(lipgloss.Color("#444444"))

		maxBullets := 15
		if totalPages <= maxBullets {
			for p := 0; p < totalPages; p++ {
				if p == m.currentPage {
					pagination.WriteString(activeBullet.Render("●"))
				} else {
					pagination.WriteString(inactiveBullet.Render("○"))
				}
				if p < totalPages-1 {
					pagination.WriteString(" ")
				}
			}
		} else {
			// Abbreviated
			if m.currentPage > 1 {
				pagination.WriteString(inactiveBullet.Render("○ ... "))
			}
			if m.currentPage > 0 {
				pagination.WriteString(inactiveBullet.Render("○ "))
			}
			pagination.WriteString(activeBullet.Render("●"))
			if m.currentPage < totalPages-1 {
				pagination.WriteString(inactiveBullet.Render(" ○"))
			}
			if m.currentPage < totalPages-2 {
				pagination.WriteString(inactiveBullet.Render(" ... ○"))
			}
		}
		pagination.WriteString(fmt.Sprintf("  %d/%d", m.currentPage+1, totalPages))
		left.WriteString("\n" + pagination.String())
	}

	// Help
	helpText := "[tab] category  [↑↓/k/space]  [←/→/pgup/pgdn] scroll  [enter] copy  [f] folder  [w/s] web  [/?] search  [q] quit"
	left.WriteString("\n" + helpStyle.Render(helpText))

	// Left panel rendering - no border
	leftPanelStyle := lipgloss.NewStyle().
		Width(leftWidth).
		Padding(0, 1, 0, 0)
	leftPanel := leftPanelStyle.Render(left.String())

	// Right panel - Documentation - use viewport for scrolling
	rightPanelStyle := lipgloss.NewStyle().
		Width(docWidth).
		Height(m.height-2).
		Padding(0, 1).
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(lipgloss.Color("#E0B7EE"))

	viewportContent := m.viewport.View()

	// Bullet pagination - like efx-face-manager
	totalLines := m.viewport.TotalLineCount()
	viewportHeight := m.viewport.Height
	scrollPercent := m.viewport.ScrollPercent()

	if totalLines > viewportHeight {
		totalPages := (totalLines + viewportHeight - 1) / viewportHeight
		currentPage := int(scrollPercent * float64(totalPages))
		if currentPage >= totalPages {
			currentPage = totalPages - 1
		}

		// Build bullets like efx-face-manager
		var pagination strings.Builder
		pagination.WriteString("  ")

		activeBullet := lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Bold(true)
		inactiveBullet := lipgloss.NewStyle().Foreground(lipgloss.Color("#444444"))

		maxBullets := 15
		if totalPages <= maxBullets {
			for p := 0; p < totalPages; p++ {
				if p == currentPage {
					pagination.WriteString(activeBullet.Render("●"))
				} else {
					pagination.WriteString(inactiveBullet.Render("○"))
				}
				if p < totalPages-1 {
					pagination.WriteString(" ")
				}
			}
		} else {
			// Abbreviated
			if currentPage > 1 {
				pagination.WriteString(inactiveBullet.Render("○ ... "))
			}
			if currentPage > 0 {
				pagination.WriteString(inactiveBullet.Render("○ "))
			}
			pagination.WriteString(activeBullet.Render("●"))
			if currentPage < totalPages-1 {
				pagination.WriteString(inactiveBullet.Render(" ○"))
			}
			if currentPage < totalPages-2 {
				pagination.WriteString(inactiveBullet.Render(" ... ○"))
			}
		}
		pagination.WriteString(fmt.Sprintf("  %d/%d", currentPage+1, totalPages))
		viewportContent = viewportContent + "\n" + pagination.String()
	}

	// Build right panel
	rightPanel := rightPanelStyle.Render(viewportContent)

	// Add toast message overlay
	if m.toast != "" {
		toastStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4")).
			Bold(true).
			Padding(0, 2)
		toast := toastStyle.Render("✓ " + m.toast)
		rightPanel = rightPanel + "\n" + toast
	}

	// Combine: left panel and right panel
	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
}

func (m model) selectedName() string {
	if m.selected != nil {
		return m.selected.name
	}
	return "Select a reference"
}

// countMarkdownFiles recursively counts .md files in a directory
func countMarkdownFiles(dir string) int {
	count := 0
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	for _, entry := range entries {
		if entry.IsDir() {
			count += countMarkdownFiles(filepath.Join(dir, entry.Name()))
		} else if strings.HasSuffix(entry.Name(), ".md") {
			count++
		}
	}
	return count
}

// generateWelcomeContent creates the welcome/readme content with stats
func generateWelcomeContent(config *Config, dataDir string) string {
	// dataDir is now the root containing docs.yaml and markdown files
	docsDir := dataDir
	fileCount := countMarkdownFiles(docsDir)
	// Subtract 1 for README.md since we show it as welcome
	if fileCount > 0 {
		fileCount--
	}

	// Count total references
	refCount := 0
	for _, cat := range config.Categories {
		refCount += len(cat.References)
	}

	// Try to load README.md
	readmePath := filepath.Join(docsDir, "README.md")
	readmeContent, _ := os.ReadFile(readmePath)

	projectName := config.Name
	if projectName == "" {
		projectName = "efx-motion"
	}

	projectDesc := config.Description
	if projectDesc == "" {
		projectDesc = "Documentation for " + projectName
	}

	// If README exists, use it as base
	if len(readmeContent) > 0 {
		content := string(readmeContent)
		// Append stats at the bottom
		content += "\n\n---\n\n"
		content += "**Stats**\n\n"
		content += fmt.Sprintf("- **%d** categories\n", len(config.Categories))
		content += fmt.Sprintf("- **%d** references\n", refCount)
		content += fmt.Sprintf("- **%d** doc files\n", fileCount)
		return content
	}

	// Fallback: generate welcome content
	content := fmt.Sprintf("# %s\n\n%s\n\n---\n\n**Stats**\n\n", projectName, projectDesc)
	content += fmt.Sprintf("- **%d** categories\n", len(config.Categories))
	content += fmt.Sprintf("- **%d** references\n", refCount)
	content += fmt.Sprintf("- **%d** doc files\n\n", fileCount)
	content += "---\n\n**Quick Start**\n\n"
	content += "- Use `[tab]` to switch categories\n"
	content += "- Use `[↑/↓]` or `[j/k]` to navigate\n"
	content += "- Use `[/]` to search\n"
	content += "- Use `[pgup/pgdn]` to scroll docs\n\n"
	content += "Select a reference from the list to view its documentation."

	return content
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func loadConfig(data []byte) (*Config, error) {
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	return &config, nil
}

func createItems(config *Config) []item {
	var items []item

	// Add README as first item
	items = append(items, item{
		name:        "README",
		description: "Project overview and stats",
		category:    "Overview",
	})

	for _, cat := range config.Categories {
		for _, ref := range cat.References {
			items = append(items, item{
				name:        ref.Name,
				description: ref.Description,
				category:    cat.Name,
			})
		}
	}
	return items
}

func main() {
	// Load workspace configuration
	configDir = getConfigDir()
	workspaceConfig, err := LoadWorkspaceConfig()
	if err != nil {
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")).Render("Error: Failed to load workspace config: " + err.Error()))
		fmt.Println("Please create " + filepath.Join(configDir, "workspaces.yaml"))
		os.Exit(1)
	}

	// Select workspace using gum
	currentWorkspace = SelectWorkspace(workspaceConfig)
	if currentWorkspace == nil {
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")).Render("Error: No workspace selected"))
		os.Exit(1)
	}

	// Read config from external file
	dataDir := getDataDir()
	configPath := filepath.Join(dataDir, "docs.yaml")

	configData, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")).Render("Error: Failed to read config at " + configPath))
		os.Exit(1)
	}

	config, err := loadConfig(configData)
	if err != nil {
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")).Render("Error: " + err.Error()))
		os.Exit(1)
	}

	// Store config globally for web navigation
	currentConfig = config

	items := createItems(config)

	// Initialize global glamour renderer once at startup
	initGlamour(60)

	// Generate welcome content with stats
	welcomeContent := generateWelcomeContent(config, dataDir)

	// Render welcome message using RenderMarkdown (uses dark style)
	welcomeRendered := RenderMarkdown(welcomeContent, 60)
	if welcomeRendered == "" {
		welcomeRendered = welcomeContent
	}

	// Initialize viewport
	vp := viewport.New(50, 20)
	vp.SetContent(welcomeRendered)

	// Initialize paginator for bullet dots
	pager := paginator.New()
	pager.Type = paginator.Dots
	pager.ActiveDot = lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Render("●")
	pager.InactiveDot = lipgloss.NewStyle().Foreground(lipgloss.Color("#444444")).Render("•")

	m := model{
		config:        *config,
		items:         items,
		filteredItems: items,
		cursor:        0,
		activeTab:     0,
		docContent:    welcomeContent,
		viewport:      vp,
		paginator:     pager,
		docCache:      map[string]string{"welcome": welcomeRendered},
		docCacheKey:   "welcome",
	}

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if err := p.Start(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}

// generateSidebarHTML creates the sidebar navigation HTML
func generateSidebarHTML(activeCat, activeDoc string) string {
	if currentConfig == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<div class="sidebar"><div class="sidebar-header">efx-motion Docs <span style="font-size:12px;color:#666">%s</span></div>`, Version))

	for _, cat := range currentConfig.Categories {
		catActive := ""
		if cat.Name == activeCat {
			catActive = " active"
		}
		sb.WriteString(fmt.Sprintf(`<div class="category%s"><div class="cat-title" onclick="toggleCat(this)">▶ %s</div><div class="cat-items">`, catActive, cat.Name))
		for _, ref := range cat.References {
			docActive := ""
			if ref.Name == activeDoc {
				docActive = " class=\"active\""
			}
			sb.WriteString(fmt.Sprintf(`<a href="/?cat=%s&doc=%s"%s>%s</a>`,
				urlEncode(cat.Name), urlEncode(ref.Name), docActive, ref.Name))
		}
		sb.WriteString(`</div></div>`)
	}
	sb.WriteString(`<div class="sidebar-footer">[j/k] navigate • [enter] open • [r] refresh • [q] close</div></div>`)
	return sb.String()
}

func urlEncode(s string) string {
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "&", "%26")
	s = strings.ReplaceAll(s, "#", "%23")
	return s
}

// generateFullPageHTML creates the full page with sidebar
func generateFullPageHTML(title, content, activeCat, activeDoc string) string {
	sidebar := generateSidebarHTML(activeCat, activeDoc)

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
	<meta charset="utf-8">
	<title>%s - efx-motion</title>
	<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/styles/github-dark.min.css" id="dark-hl">
	<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/styles/github.min.css" id="light-hl" disabled>
	<script src="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/highlight.min.js"></script>
	<script>hljs.highlightAll();</script>
	<style>
		* { box-sizing: border-box; margin: 0; padding: 0; }
		body {
			font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif;
			background: #0d1117;
			color: #c9d1d9;
			display: flex;
			height: 100vh;
			overflow: hidden;
		}
		body.light {
			background: #ffffff;
			color: #24292f;
		}
		.theme-toggle {
			position: fixed;
			top: 10px;
			right: 20px;
			background: #7d56f4;
			color: white;
			border: none;
			padding: 8px 16px;
			border-radius: 20px;
			cursor: pointer;
			font-size: 12px;
			z-index: 1000;
		}
		.theme-toggle:hover { opacity: 0.9; }
		body.light .theme-toggle { background: #7d56f4; }
		.sidebar {
			width: 280px;
			background: #161b22;
			border-right: 1px solid #30363d;
			display: flex;
			flex-direction: column;
			overflow: hidden;
		}
		body.light .sidebar {
			background: #f6f8fa;
			border-right: 1px solid #d0d7de;
		}
		.sidebar-header {
			padding: 16px;
			font-size: 18px;
			font-weight: bold;
			color: #7d56f4;
			border-bottom: 1px solid #30363d;
		}
		body.light .sidebar-header { border-bottom: 1px solid #d0d7de; }
		.sidebar-footer {
			padding: 12px;
			font-size: 11px;
			color: #8b949e;
			border-top: 1px solid #30363d;
			text-align: center;
		}
		body.light .sidebar-footer { color: #57606a; border-top-color: #d0d7de; }
		.category { border-bottom: 1px solid #21262d; }
		body.light .category { border-bottom: 1px solid #d0d7de; }
		.cat-title {
			padding: 10px 16px;
			cursor: pointer;
			font-weight: 600;
			color: #f0f6fc;
			transition: background 0.2s;
		}
		body.light .cat-title { color: #24292f; }
		.cat-title:hover { background: #21262d; }
		body.light .cat-title:hover { background: #f3f4f6; }
		.category.active .cat-title { color: #7d56f4; }
		.cat-items { display: none; background: #0d1117; }
		body.light .cat-items { background: #ffffff; }
		.category.active .cat-items { display: block; }
		.cat-items a {
			display: block;
			padding: 8px 16px 8px 32px;
			color: #8b949e;
			text-decoration: none;
			font-size: 13px;
			transition: all 0.2s;
		}
		body.light .cat-items a { color: #57606a; }
		.cat-items a:hover { background: #21262d; color: #c9d1d9; }
		body.light .cat-items a:hover { background: #f3f4f6; color: #24292f; }
		.cat-items a.active { background: #7d56f420; color: #7d56f4; border-right: 2px solid #7d56f4; }
		.content {
			flex: 1;
			overflow-y: auto;
			padding: 40px 60px;
			max-width: calc(100%% - 280px);
			width: 100%%;
		}
		body.light .content { color: #24292f; }
		pre { background: #161b22; padding: 16px; border-radius: 8px; overflow-x: auto; }
		body.light pre { background: #f6f8fa; border: 1px solid #d0d7de; }
		code { font-family: 'Fira Code', 'Monaco', 'Menlo', monospace; font-size: 14px; background: #30363d; color: #e6edf3; padding: 2px 4px; border-radius: 4px; }
		body.light code { background: #f6f8fa; color: #24292f; }
		pre code { padding: 0; background: none; }
		a { color: #58a6ff; text-decoration: none; }
		a:hover { text-decoration: underline; }
		h1, h2, h3, h4 { color: #f0f6fc; margin: 24px 0 16px; }
		body.light h1, body.light h2, body.light h3, body.light h4 { color: #24292f; }
		h1 { font-size: 2em; border-bottom: 1px solid #30363d; padding-bottom: 10px; }
		h2 { font-size: 1.5em; border-bottom: 1px solid #30363d; padding-bottom: 8px; }
		body.light h1, body.light h2 { border-bottom-color: #d0d7de; }
		blockquote { border-left: 4px solid #7d56f4; margin: 16px 0; padding: 0 16px; color: #8b949e; }
		body.light blockquote { color: #57606a; }
		ul, ol { padding-left: 24px; }
		li { margin: 8px 0; }
		table { border-collapse: collapse; width: 100%%; margin: 16px 0; }
		th, td { border: 1px solid #30363d; padding: 10px 14px; text-align: left; }
		body.light th, body.light td { border-color: #d0d7de; }
		th { background: #161b22; }
		body.light th { background: #f6f8fa; }
		hr { border: none; border-top: 1px solid #30363d; margin: 32px 0; }
		body.light hr { border-top-color: #d0d7de; }
	</style>
</head>
<body>
%s
<button class="theme-toggle" onclick="toggleTheme()">Light</button>
<div class="content">
%s
</div>
<script>
	// Theme toggle
	function toggleTheme() {
		document.body.classList.toggle('light');
		var btn = document.querySelector('.theme-toggle');
		if (document.body.classList.contains('light')) {
			btn.textContent = 'Dark';
			document.getElementById('dark-hl').disabled = true;
			document.getElementById('light-hl').disabled = false;
		} else {
			btn.textContent = 'Light';
			document.getElementById('dark-hl').disabled = false;
			document.getElementById('light-hl').disabled = true;
		}
		localStorage.setItem('theme', document.body.classList.contains('light') ? 'light' : 'dark');
	}
	// Load saved theme
	if (localStorage.getItem('theme') === 'light') {
		document.body.classList.add('light');
		document.querySelector('.theme-toggle').textContent = 'Dark';
		document.getElementById('dark-hl').disabled = true;
		document.getElementById('light-hl').disabled = false;
	}
	function toggleCat(el) { el.parentElement.classList.toggle('active'); }
	let currentIdx = 0;
	const links = document.querySelectorAll('.cat-items a');
	links.forEach((link, idx) => { if (link.classList.contains('active')) currentIdx = idx; });
	document.addEventListener('keydown', (e) => {
		if (e.key === 'j' || e.key === 'ArrowDown') {
			currentIdx = Math.min(currentIdx + 1, links.length - 1);
			links[currentIdx].click();
		} else if (e.key === 'k' || e.key === 'ArrowUp') {
			currentIdx = Math.max(currentIdx - 1, 0);
			links[currentIdx].click();
		} else if (e.key === 'Enter' || e.key === 'r') {
			location.reload();
		} else if (e.key === 'q') {
			window.close();
		}
	});
</script>
</body>
</html>`, title, sidebar, content)

	return html
}

// updateWebPreview updates the web preview with new content
func updateWebPreview(docName, catName, content string) {
	if httpServer == nil {
		return
	}

	currentDocName = docName
	currentCatName = catName

	// Convert markdown to HTML
	var buf strings.Builder
	if err := webMarkdown.Convert([]byte(content), &buf); err != nil {
		buf.WriteString(content)
	}
	htmlContent := buf.String()

	// Generate full page
	currentHTML = generateFullPageHTML(docName, htmlContent, catName, docName)
}

// serveMarkdown starts the HTTP server with full page navigation
func serveMarkdown(title, content, catName, docName string) {
	currentDocName = docName
	currentCatName = catName

	// Convert markdown to HTML
	var buf strings.Builder
	if err := webMarkdown.Convert([]byte(content), &buf); err != nil {
		buf.WriteString(content)
	}
	htmlContent := buf.String()

	// Generate full page
	currentHTML = generateFullPageHTML(title, htmlContent, catName, docName)

	// If server already running, just return (content updated)
	if httpServer != nil {
		return
	}

	// Start server
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")

		// Parse URL params
		params := r.URL.Query()
		docName := params.Get("doc")
		catName := params.Get("cat")

		var content string
		var html string

		if docName != "" {
			// Load document from disk
			content = loadDocContentFromDisk(catName, docName)
			if content != "" {
				// Convert markdown to HTML
				var buf strings.Builder
				if err := webMarkdown.Convert([]byte(content), &buf); err != nil {
					buf.WriteString(content)
				}
				html = generateFullPageHTML(docName, buf.String(), catName, docName)
			}
		}

		// If no valid doc, use current content
		if html == "" {
			html = currentHTML
		}

		fmt.Fprint(w, html)
	})

	httpServer = &http.Server{Addr: ":8080", Handler: mux}

	// Open browser
	go func() {
		time.Sleep(300 * time.Millisecond)
		exec.Command("open", "http://localhost:8080").Start()
	}()

	go func() {
		httpServer.ListenAndServe()
	}()
}

// loadDocContentFromDisk loads markdown content from disk based on category and doc name
func loadDocContentFromDisk(catName, docName string) string {
	if catName == "" || docName == "" {
		return ""
	}

	// Map category names to folder names
	categoryMap := map[string]string{
		"Core":       "core",
		"Responsive": "responsive",
		"Helpers":    "helpers",
		"Components": "components",
		"Templates":  "templates",
		"Player":     "player",
	}

	folder := categoryMap[catName]
	if folder == "" {
		folder = "core"
	}

	dataDir := getDataDir()
	docsDir := filepath.Join(dataDir, folder)

	// Try different file name variations
	possibleNames := []string{
		docName + ".md",
		strings.ReplaceAll(docName, " ", "-") + ".md",
		strings.ReplaceAll(docName, " ", "") + ".md",
		strings.ReplaceAll(docName, "-", " ") + ".md",
		strings.ReplaceAll(docName, "-", "") + ".md",
	}

	for _, fileName := range possibleNames {
		fullPath := filepath.Join(docsDir, fileName)
		content, err := os.ReadFile(fullPath)
		if err == nil && len(content) > 0 {
			return string(content)
		}
	}

	return ""
}

// Keep list import used
var _ key.Binding
