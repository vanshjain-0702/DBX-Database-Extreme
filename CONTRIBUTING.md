# Contributing to DBX

Thank you for your interest in contributing! We welcome bug reports, feature requests, documentation improvements, and code contributions.

## Before you propose a feature

Read [docs/positioning.md](docs/positioning.md). DBX is deliberately narrow: it is the
per-tenant memory engine for AI products, and there is an explicit list of things we do not
try to be. A feature is much easier to get merged when the pull request says which of the four
USPs it strengthens. Proposals that pull DBX toward being a general-purpose database, or that
chase a benchmark against another product, will usually be declined — not because they are bad
ideas, but because they belong in a different project.

The same applies to documentation: please follow the language guide in §9 of that document.
We do not describe DBX as a replacement for anything.

## Getting Started

1. **Fork** the repository and clone your fork.
2. **Create a branch** for your change: `git checkout -b feat/my-feature`
3. **Make your changes** and ensure all tests pass: `make test`
4. **Submit a Pull Request** against the `main` branch.

## Development Setup

```bash
# Clone
git clone https://github.com/vanshjain-0702/DBX-Database-Extreme.git
cd DBX-Database-Extreme

# Install Go dependencies
go mod download

# Run tests
make test

# Operator drills (not a CI default)
make soak            # 100 idle / 25 active KV engines
make restore-drill   # backup/restore + hibernate + usage tests

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
