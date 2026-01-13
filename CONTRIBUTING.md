# Contributing to Golang-Oblivion

This document provides guidelines for contributing to the project.

## Code of Conduct

- Be respectful and constructive
- Focus on what is best for the community
- Show empathy towards others

## Development Setup

1. **Fork and Clone**
   ```bash
   git clone https://github.com/sewaeth/Golang-Oblivion
   cd Golang-Oblivion
   ```

2. **Install Dependencies**
   ```bash
   go mod download
   ```

3. **Set Up Environment**
   ```bash
   cp .env.example .env
   # Edit .env with your bot token
   ```

## Development Workflow

1. **Create a Branch**
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Make Changes**
   - Write clean, well-documented code
   - Follow Go conventions and idioms
   - Add comments for complex logic

3. **Test Your Changes**
   ```bash
   make lint      # Format and vet
   make build     # Ensure it builds
   make test      # Run tests (when available)
   ```

4. **Commit**
   ```bash
   git add .
   git commit -m "feat: add your feature description"
   ```

   **Commit Message Format:**
   - `feat:` New feature
   - `fix:` Bug fix
   - `docs:` Documentation changes
   - `refactor:` Code refactoring
   - `test:` Adding tests
   - `chore:` Maintenance tasks

5. **Push and Create PR**
   ```bash
   git push origin feature/your-feature-name
   ```

## Code Standards

### Go Style
- Follow [Effective Go](https://golang.org/doc/effective_go.html)
- Use `gofmt` for formatting (included in `make lint`)
- Keep functions small and focused
- Use meaningful variable names

### Structured Logging
Use `slog` for all logging:
```go
// Good
slog.Info("Operation completed", "user_id", userID, "duration", elapsed)

// Bad
log.Printf("Operation completed for %s in %v", userID, elapsed)
```

### Error Handling
```go
// Good - structured error with context
if err != nil {
    return fmt.Errorf("failed to load config: %w", err)
}

// Bad - unstructured error
if err != nil {
    return err
}
```

### Input Validation
Always validate user inputs:
```go
if value < 1 || value > 100 {
    return fmt.Errorf("value must be between 1 and 100")
}
```

## Testing Guidelines

When adding new features:
1. Write unit tests for new functions
2. Ensure tests pass with `make test`
3. Check for race conditions with race detector

## Pull Request Process

1. **Update Documentation**
   - Update README.md if adding features
   - Add inline comments for complex code

2. **Ensure Quality**
   - All lint checks pass
   - Code builds successfully
   - No new warnings from `go vet`

3. **Describe Changes**
   - Clear PR title and description
   - List what changed and why
   - Reference any related issues

4. **Review Process**
   - Maintainers will review your PR
   - Address feedback promptly
   - Be open to suggestions

## What to Contribute

### Good First Issues
- Documentation improvements
- Adding validation for edge cases
- Improving error messages
- Adding configuration examples

### Feature Ideas
- Additional webhook modes
- Enhanced rate limiting
- Prometheus metrics
- Health check endpoints

### Bug Reports
Include:
- Steps to reproduce
- Expected vs actual behavior
- Go version and OS
- Relevant log output

## Questions?

Open an issue or reach out to maintainers.

Thank you for your interest in contributing! <3