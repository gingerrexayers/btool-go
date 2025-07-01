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

### Internal Design: The ObjectStore

At the heart of `btool`'s storage layer is the `ObjectStore`, an instance-based and thread-safe component responsible for managing all data objects (chunks, manifests, and trees). Previously a collection of global functions, the object store was refactored into an encapsulated `ObjectStore` struct to eliminate global state and prevent race conditions during concurrent operations.

Key characteristics of the new design:

-   **Encapsulation**: All object store state, including the in-memory index of pending objects and file locks, is managed within an `ObjectStore` instance. This prevents state from leaking and ensures that each command (`snap`, `restore`, `prune`) operates on an isolated, consistent view of the repository.
-   **Thread Safety**: A mutex within the `ObjectStore` struct protects against data corruption when objects are written concurrently, a common scenario during the `snap` process.
-   **Explicit Commits**: Objects are written to a temporary in-memory map and are only persisted to disk when the `Commit()` method is called. This atomic operation ensures that the on-disk index and packfiles are never left in an inconsistent state.

This robust design was implemented to resolve a critical bug where snapshot restores would fail due to missing objects in the index. By centralizing state management, the new `ObjectStore` guarantees the integrity and reliability of the backup repository.

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

**Flags:**
-   `-m, --message string`: A message to associate with the snap.

**Usage:**
```sh
# Create a snap of the current directory with a message
btool snap -m "Initial backup of my project"

# Create a snap of a different directory
btool snap /path/to/my/other/project -m "Backup of other project"
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
SNAPSHOT   HASH       TIMESTAMP                  SOURCE SIZE     SNAP SIZE       MESSAGE
========   ========   ========================   =============   =============   =======
1          f4a9b1c    2023-10-27 10:30:05 UTC    1.25 MB         1.10 MB         Initial commit
2          9e1d3a8    2023-10-28 15:12:45 UTC    1.28 MB         1.12 MB         Added new feature
3          c3b0a2f    2023-10-29 11:05:19 UTC    1.35 MB         1.18 MB         Refactored core logic

Total stored size of all objects: 1.18 MB
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

Safely removes old snapshots and performs garbage collection to free up storage space.

The `prune` command keeps the snapshot specified by `<snap-identifier>` and **all snapshots created after it**. Any snapshots created *before* the specified one will be permanently deleted. After removing the old snapshot records, it scans the repository for data chunks that are no longer referenced by any of the remaining snapshots and deletes them.

The snapshot identifier can be a numeric ID (from `btool list`) or a unique hash prefix.

**Arguments:**
-   `<snap-identifier>`: (Required) The ID or hash prefix of the oldest snapshot **to keep**.
-   `[directory]`: (Optional) The path to the project directory. Defaults to the current directory.

**Example:**

Imagine your snapshot list looks like this:
```
$ btool list
SNAPSHOT   HASH       TIMESTAMP                  SOURCE SIZE     SNAP SIZE       MESSAGE
========   ========   ========================   =============   =============   =======
1          f4a9b1c    2023-10-27 10:30:05 UTC    1.25 MB         1.10 MB         Initial commit
2          9e1d3a8    2023-10-28 15:12:45 UTC    1.28 MB         1.12 MB         Added new feature
3          c3b0a2f    2023-10-29 11:05:19 UTC    1.35 MB         1.18 MB         Refactored core logic
4          a1b2c3d    2023-10-30 18:00:00 UTC    1.40 MB         1.25 MB         Final touches
```

If you run `btool prune 3`, snapshot `3` and `4` will be kept, while `1` and `2` will be removed.

```sh
# Keep snapshot 3 and all newer ones, prune everything older.
$ btool prune 3
🧹 Starting prune for "/Users/mark/work/btool-go", removing snaps older than 3...
   - Marking live objects from snapshots to keep...
   - Sweeping old objects and rebuilding index...
   - Finalizing changes...
✅ Prune complete!
   - Deleted 2 old snap(s).
```

After pruning, the list will only show the remaining snapshots:
```
$ btool list
SNAPSHOT   HASH       TIMESTAMP                  SOURCE SIZE     SNAP SIZE       MESSAGE
========   ========   ========================   =============   =============   =======
3          c3b0a2f    2023-10-29 11:05:19 UTC    1.35 MB         1.18 MB         Refactored core logic
4          a1b2c3d    2023-10-30 18:00:00 UTC    1.40 MB         1.25 MB         Final touches
```

You can also use a hash prefix:
```sh
# This would have the same effect as 'btool prune 3'
btool prune c3b0a2f
```

### Tab Completion

`btool` supports generating shell completion scripts for Bash, Zsh, Fish, and PowerShell. This allows you to get suggestions for commands and arguments (like snapshot IDs) by pressing the `Tab` key.

The following examples show how to load completions for your current session and how to make them permanent.

**Bash:**

To load completions for the current session, run:
```sh
source <(btool completion bash)
```

To load completions for all new sessions, run the appropriate command for your OS once:
```sh
# macOS (requires Homebrew and bash-completion)
# If you don't have bash-completion, install it: brew install bash-completion
btool completion bash > $(brew --prefix)/etc/bash_completion.d/btool

# Linux
sudo btool completion bash > /etc/bash_completion.d/btool
```

**Zsh:**

If shell completion is not already enabled in your environment, you will need to enable it by running the following command once:
```sh
echo "autoload -U compinit; compinit" >> ~/.zshrc
```

To load completions for all new sessions, run this command once:
```sh
btool completion zsh > "${fpath[1]}/_btool"
```
You will need to start a new shell for this setup to take effect.

**Fish:**

To load completions for the current session, run:
```sh
btool completion fish | source
```

To load completions for all new sessions, run this command once:
```sh
btool completion fish > ~/.config/fish/completions/btool.fish
```

**PowerShell:**

To load completions for the current session, run:
```powershell
btool completion powershell | Out-String | Invoke-Expression
```

To load completions for all new sessions, add the following line to your PowerShell profile:
```powershell
Invoke-Expression (& btool completion powershell | Out-String)
```
If you're not sure where your profile file is, you can find its path by running `echo $PROFILE`. If the file doesn't exist, you can create it.

Once enabled, you can type a command and press `Tab` to see suggestions:
```sh
$ btool prune <TAB>
1   f4a9b1c 2023-10-27 10:30:05 - Initial commit
2   9e1d3a8 2023-10-28 15:12:45 - Added new feature
3   c3b0a2f 2023-10-29 11:05:19 - Refactored core logic
4   a1b2c3d 2023-10-30 18:00:00 - Final touches
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