# btool-go

`btool-go` is a simple, efficient, and reliable command-line file backup tool written in Go. It is inspired by concepts from version control systems like Git and modern backup software like Restic. It creates content-addressable, de-duplicated snapshots of your directories, ensuring that data is stored efficiently and safely.

## Key Features

-   **Content-Addressable Storage**: Files are broken into variable-sized chunks. Each chunk is identified by the hash of its content, meaning identical data (even across different files or snapshots) is stored only once.
-   **Efficient Chunking**: Uses Rabin fingerprinting to determine chunk boundaries. This is highly effective at minimizing the amount of new data that needs to be stored when files are modified.
-   **Point-in-Time Snapshots**: Easily create immutable snapshots (`snaps`) of your directory's state at any time.
-   **`.btoolignore` Support**: Exclude files and directories from your snapshots using a familiar `.gitignore` style syntax.
-   **Garbage Collection**: The `prune` command safely removes old snapshots and deletes any data chunks that are no longer referenced, freeing up storage space.
-   **Cross-Platform**: Built with Go, `btool` is a single, self-contained binary that runs on Linux, macOS, and Windows.

## How It Works

When you create a snapshot (`snap`) of a directory, `btool` performs the following steps:
1.  It scans the directory, ignoring any paths specified in a `.btoolignore` file.
2.  Each file is split into variable-sized data chunks.
3.  Each chunk is hashed (SHA-256). The hash becomes the chunk's unique identifier.
4.  The tool creates a manifest for each file, listing the hashes of the chunks that make it up.
5.  A tree object is created for each directory, listing the files and subdirectories it contains.
6.  All these objects (chunks, manifests, trees) are stored in a `.btool/packs` directory. Because objects are identified by their content hash, de-duplication is automatic.
7.  Finally, a single `snap` file is created in `.btool/snaps`, pointing to the root tree hash and containing metadata like the creation time and a message.

This creates a hidden `.btool` directory at the root of your project:

```
your-project/
├── .btool/
│   ├── index.json   # Maps object hashes to their location in a packfile
│   ├── packs/       # Contains the actual data chunks, packed together
│   └── snaps/       # Contains small JSON files defining each snapshot
├── .btoolignore     # (Optional) Your file to specify ignore patterns
├── file1.txt
└── subdir/
    └── file2.txt
```

---

## Installation

### Prerequisites

You need to have **Go (version 1.23 or newer)** installed on your system.

### Option 1: Using `go install` (Recommended)

This is the simplest way to install the `btool` binary to your system's `GOPATH`.

```sh
go install github.com/gingerrexayers/btool-go/cmd/btool@latest
```

Ensure that your Go bin directory (e.g., `~/go/bin`) is in your system's `PATH`.

### Option 2: Building from Source

If you prefer to build the binary yourself:

1.  **Clone the repository:**
    ```sh
    git clone https://github.com/gingerrexayers/btool-go.git
    cd btool-go
    ```

2.  **Build the binary:**
    ```sh
    go build -o btool ./cmd/btool
    ```
    This will create a `btool` executable in the current directory.

3.  **Install it (optional):**
    You can move this binary to a location in your `PATH` to make it accessible system-wide.
    ```sh
    # For Linux/macOS
    sudo mv btool /usr/local/bin/

    # For Windows (using PowerShell as an Administrator)
    # Move-Item -Path .\btool.exe -Destination "C:\Program Files\btool\"
    # (Ensure the destination is in your system's PATH)
    ```

---

## Usage

`btool` is a command-line tool. You can run commands against the current directory or specify a target directory.

### `btool snap [directory]`

Creates a new snapshot of the specified directory (or the current directory if none is provided).

**Usage:**
```sh
# Create a snapshot of the current directory
btool snap

# Create a snapshot of a specific project
btool snap /path/to/my/project
```

### `btool list [directory]`

Lists all available snapshots for a repository, sorted chronologically. Each snap is given a sequential ID for easy reference.

**Usage:**
```sh
btool list
```

**Example Output:**
```
Snaps for "/Users/mark/work/btool-go":
SNAPSHOT   HASH       TIMESTAMP                  SOURCE SIZE     MESSAGE
========   ========   ========================   =============   =======
1          f4a9b1c    2023-10-27 10:30:05 UTC    1.25 MB         Initial commit
2          9e1d3a8    2023-10-28 15:12:45 UTC    1.28 MB         Added new feature
3          c3b0a2f    2023-10-29 11:05:19 UTC    1.35 MB         Refactored core logic

Total stored size of all objects: 950.75 KB
```

### `btool restore <snap_id_or_hash>`

Restores a directory's state from a specific snapshot. You can identify the snapshot by its **ID** (from `btool list`) or by a **unique prefix of its hash**.

**Flags:**
-   `-d, --directory <path>`: The source directory containing the `.btool` repository (defaults to current directory).
-   `-o, --output <path>`: The directory to restore files to. **If not specified, it will restore in-place, overwriting the source directory.**

**Usage:**
```sh
# Restore snapshot with ID 2 to a new directory
btool restore 2 -o ./my-restore-destination

# Restore a snapshot using a hash prefix from a different source directory
btool restore c3b0a2f --directory /path/to/my/project -o /tmp/restored_project

# DANGER: Restore in-place, overwriting the current directory's files
btool restore 1
```

### `btool prune <snap-identifier> [directory]`

Removes snapshots older than the one specified and deletes unreferenced data objects to free up storage space. The snapshot identifier can be a numeric ID (e.g., `3`) or a unique hash prefix (e.g., `c3b0a2f`).

**Arguments:**
-   `<snap-identifier>`: (Required) The ID or hash prefix of the oldest snapshot to keep. All snapshots created *before* this one will be removed.
-   `[directory]`: (Optional) The path to the project directory. Defaults to the current directory.

**Usage:**
```sh
# Prune all snapshots older than snapshot with ID 3
btool prune 3

# Prune all snapshots older than the one with the specified hash prefix
btool prune c3b0a2f

# Prune snapshots in a different project directory
btool prune 10 /path/to/my/project
```

---

## For Developers

### Building from Source

To build the binary locally for development:
```sh
go build ./cmd/btool
```

### Running Tests

To run the complete test suite:
```sh
go test ./...
```

To run tests for a single package with verbose output:
```sh
go test -v ./internal/btool/lib
```

### Code Formatting

This project uses the standard Go formatter.
```sh
go fmt ./...
```