# Contributing to perfgo

Thank you for your interest in contributing to perfgo!

## Development Setup

### Prerequisites

- Go 1.24 or later
- golangci-lint (for linting)
- Docker (optional, for building container images)

### Local Development

1. Clone the repository:
```bash
git clone https://github.com/perfgo/perfgo.git
cd perfgo
```

2. Install dependencies:
```bash
go mod download
```

3. Run tests:
```bash
go test ./...
```

4. Run linter:
```bash
golangci-lint run
```

5. Build the binary:
```bash
go build -o perfgo .
```

## Continuous Integration

### CI Workflow

The CI workflow runs on every push and pull request to the `main` branch:

- **Tests**: Runs on Go 1.24.x and 1.25.x
  - Unit tests with race detection
  - Coverage reporting to Codecov

- **Lint**: Runs golangci-lint with all enabled linters

- **Build**: Cross-compiles for:
  - Linux (amd64, arm64)
  - macOS (amd64, arm64)

### GoReleaser Check

When you modify `.goreleaser.yaml` or `Dockerfile`, the GoReleaser check workflow:
- Validates the GoReleaser configuration
- Builds a snapshot to ensure the release process works

## Release Process

Releases are automated using GoReleaser and GitHub Actions.

### Creating a Release

1. Ensure all tests pass and code is merged to `main`

2. Create and push a version tag:
```bash
git tag v1.0.0
git push origin v1.0.0
```

3. GitHub Actions will automatically:
   - Run tests and linting
   - Build binaries for all platforms
   - Create Docker images (amd64, arm64)
   - Generate checksums
   - Create a GitHub release with changelog
   - Upload release artifacts

### Version Numbering

We follow [Semantic Versioning](https://semver.org/):
- `v1.0.0` - Major release (breaking changes)
- `v1.1.0` - Minor release (new features, backward compatible)
- `v1.0.1` - Patch release (bug fixes)

## Pull Request Guidelines

1. **Branch naming**: Use descriptive branch names
   - `feat/add-new-feature`
   - `fix/bug-description`
   - `docs/update-readme`

2. **Commit messages**: Use conventional commits format
   - `feat: add new profiling mode`
   - `fix: resolve race condition in parser`
   - `docs: update installation instructions`
   - `chore: update dependencies`

3. **Testing**: Ensure all tests pass
   ```bash
   go test ./...
   ```

4. **Linting**: Fix all linting issues
   ```bash
   golangci-lint run
   ```

5. **Documentation**: Update README.md and relevant docs

## Code Style

- Follow standard Go conventions
- Use `gofmt` for formatting
- Write meaningful variable and function names
- Add comments for exported functions and complex logic
- Keep functions focused and small

## Testing

- Write tests for new features
- Maintain or improve code coverage
- Use table-driven tests where appropriate
- Test edge cases and error conditions

## Getting Help

- Open an issue for bugs or feature requests
- Start a discussion for questions or ideas
- Check existing issues and discussions first

## License

By contributing, you agree that your contributions will be licensed under the project's license.
