# Contributing to DBX

Thank you for your interest in contributing! We welcome bug reports, feature requests, documentation improvements, and code contributions.

## Getting Started

1. **Fork** the repository and clone your fork.
2. **Create a branch** for your change: `git checkout -b feat/my-feature`
3. **Make your changes** and ensure all tests pass: `make test`
4. **Submit a Pull Request** against the `main` branch.

## Development Setup

```bash
# Clone
git clone https://github.com/your-fork/dbx.git
cd dbx

# Install Go dependencies
go mod download

# Run tests
make test

# Run in dev mode
make run-dev

# Run dashboard
make run-dashboard
```

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`).
- Write tests for all new engine or orchestrator logic.
- Keep `internal/` packages private; expose new functionality via `pkg/`.

## Commit Messages

Use conventional commits format:
- `feat: add VRANGE vector command`
- `fix: resolve panic in HNSW on empty index`
- `docs: update architecture diagram`

## License

By submitting a pull request, you agree that your contributions will be licensed under the project's [BSL 1.1 License](LICENSE).
