# Zooxplorer

Zooxplorer is a simple TUI app to explore a ZooKeeper snapshot file.

![Zooxplorer screenshot](screenshot.png)

## Run from the command line

```bash
go run ./cmd/zooxplorer ./path/to/snapshot.file
```

The snapshot file path is required.

## Basic navigation

- `Up` / `Down`: move selection in the tree (or scroll content when content pane is focused)
- `Left` / `Right`: collapse / expand selected tree node
- `Tab`: switch focus between tree and content panes
- `Alt+Up` (Option+Up): jump to parent node in the tree
- `Ctrl+S`: open snapshot statistics dialog (press any key to close)

## Quit

- `Ctrl+C`

## What it shows

- Tree view with expandable/collapsible znodes, sorted alphabetically
- Node metadata (path, timestamps, size, zxid/cversion/owner fields)
- ACL section with ACL ID/version and decoded ACL entries
- Node content with JSON pretty-printing and gzip auto-decompression
- Scrollable content pane and a bottom status bar with key hints

## Important disclaimer

This whole application was vibecoded: all code was written by AI, and I did not review a single line of code.

This project is not meant for any serious application, and there is no guarantee that it works correctly.
