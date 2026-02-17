# Building Documentation for efx-doc

This guide explains how to create documentation that works with the efx-doc TUI application.

## Directory Structure

```
your-docs/
├── docs.yaml          # Configuration file (required)
├── README.md          # Welcome page (optional)
├── core/              # Category folder
│   ├── Installation.md
│   ├── Configuration.md
│   └── ...
├── components/       # Another category folder
│   ├── Button.md
│   └── ...
└── ...
```

## docs.yaml Format

Create a `docs.yaml` file in your docs root folder:

```yaml
name: Your Project Name
description: A brief description of your project
categories:
  - name: CategoryName
    references:
      - name: ReferenceName
        description: Brief description shown in list
```

### Example

```yaml
name: My Library
description: A awesome library for doing things
categories:
  - name: Getting Started
    references:
      - name: Installation
        description: How to install the library
      - name: Quick Start
        description: Get up and running quickly
  
  - name: Components
    references:
      - name: Button
        description: Interactive button component
      - name: Input
        description: Text input component
```

## Markdown Files

Each reference in `docs.yaml` should have a corresponding markdown file in the category folder.

### File Naming

The app looks for files with these name variations (in order):
1. Exact name: `Button.md`
2. With dashes: `button.md`
3. Without spaces: `button.md`
4. Spaces as dashes: `Button-MD.md`
5. Dashes as spaces: `Button MD.md`

### Category to Folder Mapping

| Category Name | Folder Name |
|--------------|-------------|
| Core | core |
| Components | components |
| Getting Started | getting-started |
| API Reference | api |

The folder name is converted to lowercase. If no folder matches, it defaults to `core`.

### Example

If you have:
```yaml
categories:
  - name: Components
    references:
      - name: Button
```

The app looks for:
- `components/Button.md`
- `components/button.md`
- etc.

## README.md

Place a `README.md` in the docs root folder to show as the welcome/landing page.

## Workspace Configuration

To use your docs with efx-doc, add a workspace entry in `~/.config/efx-doc/workspaces.yaml`:

```yaml
workspaces:
  - name: my-docs
    path: /path/to/your-docs
    styles:
      tui: ~/.config/efx-doc/opencode_style.json
      web: ~/.config/efx-doc/tokyo_night.json
```

## Supported Markdown Features

The TUI preview uses Glamour for rendering:
- Headings (# ## ###)
- Code blocks with syntax highlighting
- Inline code
- Lists (ordered and unordered)
- Bold and italic text
- Links
- Blockquotes
- Tables

The web preview uses Goldmark with table support.

## Tips

1. **Keep descriptions short** - They appear in a narrow column
2. **Use consistent naming** - Match your `docs.yaml` names to file names
3. **Organize by category** - Group related docs into categories
4. **Include code examples** - Use fenced code blocks with language identifiers
