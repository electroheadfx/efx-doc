# efx-doc

A beautiful terminal documentation viewer for efx-motion built with Go and Bubble Tea.

![Version](https://img.shields.io/badge/version-0.1.0-blue)
![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)
![License](https://img.shields.io/badge/license-MIT-green)

## Features

- 📚 **Two-column layout**: Navigation on the left, markdown preview on the right
- 🎨 **Beautiful rendering**: Uses glamour for rich markdown rendering with Tokyo Night theme
- 🔍 **Full-text search**: Search across all documentation
- 📱 **Responsive**: Adapts to terminal size
- 🌐 **Web preview**: Open documentation in browser with `[w]` key
- 📋 **Clipboard**: Copy documentation with `[Enter]` key
- ⌨️ **Keyboard navigation**: Full keyboard support

## Installation

### From Release

Download the latest release for your platform:

```bash
# macOS
curl -L https://github.com/efxlab/efx-doc/releases/latest/download/efx-doc-darwin-arm64 -o efx-doc
chmod +x efx-doc

# Linux
curl -L https://github.com/efxlab/efx-doc/releases/latest/download/efx-doc-linux-amd64 -o efx-doc
chmod +x efx-doc

# Windows
curl -L https://github.com/efxlab/efx-doc/releases/latest/download/efx-doc-windows-amd64.exe -o efx-doc.exe
```

### From Source

```bash
git clone https://github.com/efxlab/efx-doc.git
cd efx-doc
make build
```

## Usage

```bash
# Run the application
./efx-doc

# Or install to PATH
make install
```

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `↑/↓` or `j/k` | Navigate document list |
| `Space` | Next document |
| `Tab` | Switch category |
| `←/→` or `PgUp/PgDn` | Scroll documentation |
| `/` or `?` | Search |
| `Enter` | Copy to clipboard |
| `f` | Open folder in Finder |
| `w` | Open web preview |
| `s` | Stop web server |
| `q` | Quit |

## Web Preview

Press `w` to open documentation in your browser. The web preview features:

- Collapsible category sidebar
- Syntax highlighting for code blocks
- Auto-refresh when navigating in TUI
- Keyboard navigation (`j/k` to navigate, `r` to refresh)

## Configuration

The application reads from `data/docs.yaml` for the documentation structure and `data/docs/` folder for markdown files.

### Adding Documentation

1. Add entries to `data/docs.yaml`
2. Create corresponding markdown files in `data/docs/<category>/`

## Development

```bash
# Build for current platform
make build

# Build for all platforms
make build-all

# Run in development mode
make dev

# Run tests
make test
```

## License

MIT License - see [LICENSE](LICENSE) for details.

---

Built with ❤️ using [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Glamour](https://github.com/charmbracelet/glamour)
