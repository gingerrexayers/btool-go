# Agent Guidelines for btool-go

## Build/Test/Lint Commands
- **Build**: `go build ./cmd/btool`
- **Test all**: `go test ./...`
- **Test single package**: `go test ./internal/btool/lib`
- **Test with verbose**: `go test -v ./...`
- **Format**: `go fmt ./...`
- **Vet**: `go vet ./...`

## Code Style Guidelines
- **Language**: Go 1.23.2, uses Cobra CLI framework
- **Package structure**: `cmd/` for binaries, `internal/` for private packages
- **Imports**: Standard library first, then third-party, then local packages with blank lines between groups
- **Naming**: Use Go conventions - PascalCase for exported, camelCase for unexported
- **Comments**: Package comments start with "Package name...", function comments start with function name
- **Error handling**: Always check errors, return early on error
- **Types**: Use struct tags for JSON serialization (`json:"fieldName"`)
- **Constants**: Group related constants together with descriptive comments
- **Testing**: Use table-driven tests with descriptive test case names
- **Concurrency**: Use mutexes for shared state, prefer channels for communication
- **File permissions**: Use 0755 for directories, 0644 for files