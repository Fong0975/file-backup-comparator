# File Backup Comparator

A Windows desktop application for comparing files across multiple backup paths.

## Features

- **Multiple comparison projects** — group any number of paths into a named project
- **Local and FTP paths** — mix local filesystem directories and FTP server paths within the same project
- **Missing file detection** — shows exactly which paths are missing which files
- **Content diff** — compares SHA-256 hashes to detect modified files
- **Per-path exclusions** — glob patterns to skip files/folders that are intentionally absent
- **Expected gaps** — declare per-path patterns for files a path intentionally does not have, without hiding them from comparisons across other paths
- **Global exclusions** — app-wide exclusion patterns applied to every project on top of per-path settings
- **Large file patterns** — per-project glob patterns (or an "all files" toggle) that compare matching files by name and size only, skipping content hashing entirely; ideal for large, rarely-changing files such as videos
- **Batch compare** — toolbar **Compare** button runs any selection of projects concurrently, each showing its own real-time progress; rows disappear as projects finish and the dialog closes when the last one completes
- **Sidebar result cache** — each project shows a green (identical) or red (differences) status dot after a comparison; clicking the project name switches the result view to that project's last result without re-running
- **Hash cache** — SHA-256 results are cached by file size and modification time, so unchanged files are not re-read on subsequent runs
- **Parallel hashing** — goroutine worker pool for local files; per-FTP-server single-goroutine lanes to avoid protocol conflicts
- **Real-time progress** — two-phase indicator: file count during directory scanning, byte-based percentage during hashing; annotated with the active comparison mode (Full / Mixed / Full metadata)
- **Saved FTP server profiles** — define a server once in Settings and reuse it across any number of path entries
- **Settings backup / restore** — export and import all projects and settings as a single JSON file

---

## Prerequisites

### 1. Go 1.21+

Download from <https://go.dev/dl/> and install. Verify with:

```powershell
go version
```

### 2. C compiler (required by Fyne on Windows)

Fyne renders via OpenGL/GLFW, which needs CGO. Install **MinGW-w64** (via winlibs):

1. Download the latest GCC release from <https://winlibs.com/> — choose the **Win64 · MSVCRT** zip (e.g. `winlibs-x86_64-...-gcc-14.x.x-...zip`)
2. Extract the archive; move the `mingw64` folder to a permanent location (e.g. `C:\mingw64`)
3. Add `C:\mingw64\bin` to your **PATH** environment variable
4. Open a new terminal and verify:

```powershell
gcc --version
```

> **Note:** Do not use TDM-GCC. It has a known bug where DWARF debug sections are assigned invalid virtual addresses, causing Windows to refuse to load the compiled binary.

> **Alternative:** Install [MSYS2](https://www.msys2.org/), then run `pacman -S mingw-w64-x86_64-gcc` inside the MSYS2 shell and add `C:\msys64\mingw64\bin` to your PATH.

---

## Icon & file properties (version info)

The window / taskbar icon, the `.exe` file icon in Explorer, **and** the file
properties shown on the `.exe`'s right-click → Properties → Details tab
(File description, File version, Product name, Copyright, etc.) are all
driven by build-time files — no manual resource editor needed.

| Mechanism | Effect |
|---|---|
| `assets/Icon.png` embedded via `//go:embed` | Window and taskbar icon at runtime |
| `icon_windows_amd64.syso` in project root | `.exe` file icon **and** Details-tab properties in Windows Explorer |
| `versioninfo.json` in project root | Source config for everything in the `.syso` (edit this, not the `.syso`) |

### Updating the file description / version / copyright

Edit the fields in **`versioninfo.json`** at the project root:

| Field | Shows up as |
|---|---|
| `StringFileInfo.FileDescription` | File description |
| `StringFileInfo.FileVersion` / `FixedFileInfo.FileVersion` | File version |
| `StringFileInfo.ProductName` | Product name |
| `StringFileInfo.ProductVersion` / `FixedFileInfo.ProductVersion` | Product version |
| `StringFileInfo.CompanyName` | Company |
| `StringFileInfo.LegalCopyright` | Copyright |
| `StringFileInfo.InternalName` / `OriginalFilename` | Internal/original filename |
| `IconPath` | `.exe` icon (already points at `assets/Icon.ico`) |

After editing, regenerate the `.syso`:

```powershell
go generate ./...
```

This re-reads `versioninfo.json` and overwrites `icon_windows_amd64.syso` (via
[`goversioninfo`](https://github.com/josephspurrier/goversioninfo), fetched
on demand with `go run` — no separate install step). **`go build` does not
run this automatically** — `go generate` is a separate, manual step every
time `versioninfo.json` changes. `.\build.ps1` runs both in one command:

```powershell
.\build.ps1
```

### Updating the icon image

1. Replace `assets/Icon.png` with the new image (PNG, recommended 1024 × 1024 px).
2. Convert it to a multi-resolution `.ico` containing the 16 / 32 / 48 / 256 px
   layers (e.g. [icoconvert.com](https://icoconvert.com), ImageMagick, or GIMP)
   and overwrite `assets/Icon.ico`.
3. Run `go generate ./...` (or `.\build.ps1`) to bake the new icon into
   `icon_windows_amd64.syso`.
4. Commit `assets/Icon.png`, `assets/Icon.ico`, and `icon_windows_amd64.syso`.

---

## Build

Open PowerShell in the project directory (the folder containing `go.mod`) and run:

```powershell
# Download dependencies
go mod tidy

# Run directly (for development)
go run .

# Build a standalone .exe
go build -ldflags="-s -w -H windowsgui" -o FileCompare.exe .
```

- `-s -w` strips the symbol table and DWARF debug info, producing a smaller binary
- `-H windowsgui` suppresses the background console window for a GUI application

The resulting `FileCompare.exe` is self-contained — no Go or Fyne installation needed on the target machine.

---

## Linting

Run checks:

```powershell
golangci-lint run
```

Auto-fix:

```powershell
golangci-lint run --fix
```

---

## Run

Double-click `FileCompare.exe`, or from PowerShell:

```powershell
.\FileCompare.exe
```

---

## Usage

### Create a project

1. Click **New** in the left panel.
2. Enter a project name (e.g. "Photo Backup").
3. Click **Add Path** for each backup location and fill in:
   - **Label** — friendly name shown in results (e.g. "Main PC", "NAS", "External HDD")
   - **Type** — `Local` for a directory on this machine, `FTP` for a remote server
   - **Path** — the directory to scan (local: use Browse or type directly; FTP: the sub-path on the server, e.g. `/backups/photos`)
   - **Exclusions** — one glob pattern per line for files/folders to ignore during scanning
   - **Expected Gaps** — one glob pattern per line for files this path intentionally does not have (see [Exclusions vs Expected Gaps](#exclusions-vs-expected-gaps) below)

### Configure large file patterns (optional)

In the project editor, the **Large File Patterns** section at the bottom lets you skip content hashing for files that are large but rarely change (videos, disk images, etc.):

- **All files** checkbox — every file in this project is compared by name and size only; the hash worker pool is never invoked
- **Pattern list** — one glob pattern per line (same syntax as Exclusions); only matching files are compared by size, all others are fully hashed

When a project uses both full and metadata-only files the progress dialog labels the run **Mixed comparison**. If all files are metadata-only it shows **Full metadata comparison**.

### Set up an FTP path

FTP server credentials are managed separately from projects so the same server can be reused across multiple path entries without re-entering connection details.

1. Open **Settings** (toolbar) → **FTP** tab.
2. Click **Add** and fill in the server's domain, port (default 21), an optional display name, and optionally a username. The password is never stored — it is prompted at compare time and held only for the duration of that run.
3. Save Settings.
4. In the project editor, click **Add Path**, set **Type** to `FTP`, then choose the saved server from the **Server** dropdown. Enter the sub-path on that server (e.g. `/photos`) and a label.

> **Note:** Each FTP path is processed by a dedicated single-goroutine lane during hashing because an FTP control connection cannot handle concurrent commands. Local paths share a parallel worker pool and are not affected.

### Exclusion pattern examples

A pattern with no `/` matches anywhere in the tree, the same as a plain
`.gitignore` entry with no slash. A pattern containing `/` is instead
anchored to the path's own root and matched as a path glob, the same
convention `.gitignore` and GitHub Actions `paths:` filters use: `*` matches
within a single path segment, `**` matches any number of segments.

| Pattern | Effect |
|---|---|
| `*.tmp` | Skip all `.tmp` files, anywhere |
| `node_modules` | Skip any directory named `node_modules`, anywhere |
| `.DS_Store` | Skip macOS metadata files, anywhere |
| `Thumbs.db` | Skip Windows thumbnail cache, anywhere |
| `docs/*` | Skip only files directly inside `docs/` (not nested deeper) |
| `docs/**` | Skip everything under `docs/`, at any depth |

### Exclusions vs Expected Gaps

Both use the same glob syntax, but they affect the comparison at different stages:

| | Exclusions | Expected Gaps |
|---|---|---|
| **When applied** | During scanning — matching files are never read from this path | During comparison — matching files are not reported as Missing for this path |
| **Effect on results** | File does not appear in any result tab | File still appears in Same / Different tabs if other paths have it; just not flagged as Missing for this path |
| **Typical use** | Temporary files, caches, OS metadata you never want compared | A folder that one backup path deliberately skips (e.g. a large cache dir only backed up by one source) |

**Example:** Path A and Path B both have `photos/`, but Path B does not back up `photos/raw/`. Adding `photos/raw/**` to Path B's **Expected Gaps** stops those files from appearing in the Missing tab for Path B, while still allowing them to be compared normally between Path A and any other paths that do have them.

### Run a comparison

**Single project** — click the ▶ button next to any project in the sidebar. A progress dialog shows the current phase (scanning / comparing), the active comparison mode, and estimated completion percentage.

**Batch compare** — click **Compare** in the toolbar. A checklist of all projects appears (all pre-selected); uncheck any you want to skip, then click **Compare**. All selected projects run concurrently, each with its own progress row. Completed rows disappear one by one; the dialog closes automatically when the last project finishes. Click **Cancel** at any time to stop all running comparisons after confirmation.

After one or more comparisons have run, a **status dot** appears next to each project name in the sidebar:
- 🟢 **Green** — all files are identical across every path
- 🔴 **Red** — at least one file is missing or has different content

Click a project name (not the ▶ button) to load its most recent result into the right panel without re-running the scan. Projects that have never been compared are shown grayed-out and cannot be clicked.

Results appear in three tabs:

| Tab | Description |
|---|---|
| **Missing (N)** | Files present in at least one path but absent in others |
| **Different (N)** | Files present everywhere but with different content (hash mismatch) |
| **Same (N)** | Files present everywhere and byte-for-byte identical |

Each row has a **▾** menu with context-sensitive actions:

| Tab | Available actions |
|---|---|
| Missing | Open Folder, Exclude File, Exclude Directory, Expected Gap (File), Expected Gap (Directory), Copy (to the missing path) |
| Different | Open Folder, Replace (overwrite selected paths with one path's version) |
| Same | Open Folder |

---

## Configuration storage

All data is stored in:

```
%APPDATA%\FileCompare\config.json       # projects and settings
%APPDATA%\FileCompare\hash_cache.json   # SHA-256 hash cache (auto-managed)
```

The hash cache is updated automatically after each successful comparison and pruned to remove entries for files that no longer exist. It can be safely deleted at any time — the next run will simply re-hash everything from scratch.

---

## Project structure

```
.
├── main.go                          # Entry point; embeds assets/Icon.png at compile time
├── versioninfo.json                 # Source config for icon_windows_amd64.syso
├── icon_windows_amd64.syso          # Windows PE resource — .exe icon + file properties
├── build.ps1                        # go generate (rebuild .syso) + go build in one step
├── FyneApp.toml                     # App metadata (name, version, icon path)
├── go.mod
├── assets/
│   ├── Icon.png                     # Source icon embedded as window / taskbar icon
│   └── Icon.ico                     # Multi-resolution icon baked into icon_windows_amd64.syso
├── models/
│   ├── project.go                   # Data structures (Project, PathEntry, FTPServer, Settings) and config load/save
│   └── version.go                   # VersionInfo struct, loaded from versioninfo.json at runtime
├── core/
│   ├── source.go                    # RemoteFile interface abstracting local files and FTP files
│   ├── scanner.go                   # Recursive local directory scanner with exclusion filtering
│   ├── ftp_scanner.go               # FTP directory scanner
│   ├── hasher.go                    # SHA-256 file hasher (4 MB I/O buffer)
│   ├── hashcache.go                 # Persistent hash cache (size+modTime keyed, JSON-persisted)
│   └── comparator.go                # Parallel comparison logic: scan → pre-stat → hash → classify
└── ui/
    ├── main_window.go               # Main window layout, sidebar, toolbar, and result cache
    ├── compare_all.go               # Batch compare: project selection dialog and concurrent progress display
    ├── project_editor.go            # Project and path entry editor dialog
    ├── result_view.go               # Tabbed result display (Missing / Different / Same) with project header
    ├── result_file_ops.go           # File operations from result view (copy, replace, open folder)
    ├── format_utils.go              # Text utilities (path truncation, file size formatting)
    ├── ftp_credentials.go           # FTP credential resolution and per-run session cache
    ├── popup.go                     # StyledPopup, ShowInfoPopup, ShowConfirmPopup
    ├── native_dialog_windows.go     # Native file-picker dialog (Windows)
    ├── native_dialog_other.go       # Native file-picker dialog (non-Windows fallback)
    ├── settings.go                  # Settings dialog coordinator
    ├── settings_project_order.go    # Project order tab
    ├── settings_ftp.go              # FTP server profiles tab
    ├── settings_label.go            # Saved path labels tab
    ├── settings_exclusions.go       # Global exclusions tab
    ├── settings_backup.go           # Settings export / import / reset tab
    └── settings_about.go            # About tab (build metadata)
```
