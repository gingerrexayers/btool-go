### The Development Journey of `btool`: A Step-by-Step Summary

This document outlines the evolutionary process of building the `btool` incremental backup utility, from initial design and a simplified first implementation to overcoming technical hurdles and porting a more advanced architecture to a new language.

#### Step 1: Initial Design & Technology Evaluation

The project began with a high-level architectural design based on the principles of `git`'s content-addressable storage system.

*   **Core Concepts Defined:**
    *   **Blob:** The raw content of a file, identified by its SHA-256 hash.
    *   **Tree:** A representation of a directory's structure, mapping names to blob/tree hashes.
    *   **Snapshot:** A timestamped pointer to a root tree.

*   **Initial Storage Model:** A simple "one object per file" approach was planned for the first iteration, where each object (blob or tree) would be stored as a separate file on the filesystem, sharded by the first two characters of its hash (e.g., `.btool/objects/ab/cdef...`).

*   **Technology Evaluation:**
    *   **Node.js/Bun (The Target):** Chosen to fulfill GridUnity's focus on my Node.js expertise. The plan was to leverage Bun's high-performance I/O and modern tooling.
    *   **Go (The Alternative):** Identified as an ideal technical fit for systems-level tooling due to its excellent concurrency model and straightforward single-binary compilation.
    *   **Rust (The Powerhouse):** Considered for its peak performance and memory safety but deemed overkill for the time constraints, with a higher risk of an incomplete project due to its steeper learning curve.

**Decision:** Proceed with a TypeScript implementation on the Bun runtime, starting with the simple storage model and building complexity iteratively.

#### Step 2: Building the Initial TypeScript Implementation

The first version of `btool` was built to prove the core logic of the content-addressable model.

1.  **Foundation:** A clean project structure was established, separating `commands`, `lib`, and `types`. `commander` was initially chosen for its maturity in the Node.js ecosystem for parsing CLI arguments.
2.  **Core Services (v1):** The initial libraries were built and unit-tested:
    *   `hasher.ts`: Implemented secure, streaming SHA-256 hashing.
    *   `object-store.ts`: Implemented the simple "one object per file" sharded storage model.
3.  **Command Implementation (v1):** The `snapshot`, `list`, `restore`, and `prune` commands were implemented based on this simple architecture. Concurrency was handled by processing directory entries in parallel with `Promise.all`.
4.  **Testing:** A solid base of unit and E2E tests were written to validate the snapshot, restore, and prune round trip, ensuring the fundamental logic was correct.

#### Step 3: Architectural Evolution and Hitting a Technical Wall

With the core functionality proven, the architecture was evolved to address the limitations of the initial design and to meet more advanced requirements.

1.  **Architectural Enhancements:**
    *   **Chunking:** To efficiently back up large files, the model was upgraded to use content-defined chunking. The `rabin-wasm` library was introduced to split files into variable-sized chunks, ensuring that only modified parts of large files would result in new data being stored.
    *   **Packfiles:** To solve the inefficiency of creating thousands of small files on disk, the object store was re-architected to use a `git`-like packfile system. New objects were now buffered and committed in batches to larger "packfiles," with a single `index.json` file mapping object hashes to their locations.
    *   **Worker Threads:** To handle the CPU-intensive work of chunking and hashing files in parallel without blocking the main event loop, the `snap`, `restore`, and `prune` commands were refactored to use a pool of `Worker` threads.

2.  **The Roadblock:** After successfully implementing these advanced features, a critical issue was discovered during the final packaging phase.
    *   **The Problem:** The `bun build --compile` command, intended to create a single, standalone executable, was unable to correctly bundle the WebAssembly (`.wasm`) module required by `rabin-wasm` when that module was called from within a worker thread.
    *   **The Impact:** This meant that while the application was fully functional when run with `bun run`, a key "professional-grade" requirement—a simple, distributable binary—could not be met with the current stack.

#### Step 4: The Pivot: Porting the Advanced Architecture to Go

This technical limitation prompted a strategic pivot to Go, which is renowned for its robust single-binary compilation and first-class concurrency. The goal was to port the mature, V2 architecture with chunking and packfiles.

1.  **Translation, Not Reinvention:** The migration was a direct translation of the established architectural concepts.
    *   TypeScript `interface`s were mapped to Go `struct`s.
    *   The packfile-based `object-store` logic was ported using Go's `os` and `io` packages and protected with mutexes for thread safety.
    *   The `rabin-wasm` library was replaced with a native Go equivalent (`aclements/go-rabin`).
2.  **Idiomatic Concurrency:** The TypeScript `Worker` pool was re-implemented using Go's superior and more idiomatic concurrency primitives: **goroutines and channels**. This resulted in a cleaner and more efficient parallel processing model for the `snap`, `restore`, and `prune` commands.
3.  **CLI and Testing:**
    *   `Cobra` was chosen as the CLI framework for its power and extensibility.
    *   The project was structured following Go's best practices, cleanly separating the `cmd` (CLI adapter) from the `internal` (pure business logic) packages.
    *   The entire test suite was ported to Go's built-in `testing` package, using subtests (`t.Run`) and conventions like `_test` packages for black-box integration testing, ensuring the Go version was just as robustly verified as the original.

### Conclusion

The development of `btool` was a multi-stage journey that showcased iterative design, adaptation to technical challenges, and cross-language architectural mapping. The initial Bun/TypeScript implementation successfully validated the core backup logic and was then evolved into a sophisticated system with advanced features like content-defined chunking and packfile storage.

When a critical toolchain limitation was encountered, the project demonstrated resilience by pivoting to Go. The final Go implementation successfully translated the advanced architecture into a new ecosystem, resulting in a tool with superior performance, a more robust concurrency model, and truly seamless (and far more compact) single-binary distribution, confirming its suitability as a professional-grade systems utility.