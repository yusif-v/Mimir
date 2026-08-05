# Contributing to Mimir

Thank you for your interest in contributing! This document outlines how to get involved.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/mimir.git`
3. Create a feature branch: `git checkout -b feat/my-feature`
4. Make your changes
5. Run tests: `make test`
6. Run the linter: `make lint`
7. Commit and push: `git commit -m "feat: add my feature" && git push origin feat/my-feature`
8. Open a Pull Request

## Development Setup

### Prerequisites

- Go 1.26 or later
- Docker (optional, for running containerized analysis tools)
- GNU Make

### Building

```bash
make build          # Build the binary
make run            # Build and run
```

### Testing

```bash
make test           # Run all tests
make lint           # Run go vet
make fmt            # Format code
```

## Code Style

- Follow standard Go conventions ([Effective Go](https://go.dev/doc/effective_go))
- Run `make fmt` before committing
- Keep functions focused and well-named
- Add tests for new features

## Commit Messages

Mimir follows [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` — new feature
- `fix:` — bug fix
- `docs:` — documentation changes
- `refactor:` — code refactoring
- `test:` — adding or updating tests
- `chore:` — maintenance tasks
- `security:` — security hardening or fixes

## Reporting Issues

When reporting bugs, please include:

- Mimir version (`mimir version`)
- Go version (`go version`)
- Operating system
- Steps to reproduce
- Expected vs actual behavior

## Feature Requests

Open an issue describing the feature, why it's useful, and how it might be implemented. We'll discuss before you invest significant effort.

## Code of Conduct

Be respectful and constructive. Harassment and dismissive behavior won't be tolerated.

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
