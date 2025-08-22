# Contribution Guidelines

Thank you for your interest in contributing to HyperCache! This document outlines the process and guidelines for contributing to the project.

## Getting Started

Before you begin:

1. **Read the Documentation**: Familiarize yourself with the project by reading:
   - [README.md](../README.md) - Project overview and features
   - [Development Setup](development-setup.md) - Environment setup
   - [Code Structure](code-structure.md) - Architecture overview

2. **Check Existing Issues**: Look through [GitHub Issues](https://github.com/rishabhverma17/HyperCache/issues) to see if your idea or bug report already exists.

3. **Join the Discussion**: Feel free to start a discussion in [GitHub Discussions](https://github.com/rishabhverma17/HyperCache/discussions) for large features or architectural changes.

## How to Contribute

### Reporting Bugs

When filing a bug report, please include:

1. **Clear Description**: What happened vs. what you expected
2. **Steps to Reproduce**: Minimal steps to reproduce the issue
3. **Environment**: Go version, OS, HyperCache version
4. **Configuration**: Relevant config file sections
5. **Logs**: Error messages and relevant log output

**Bug Report Template:**
```markdown
**Bug Description**
A clear description of what the bug is.

**To Reproduce**
Steps to reproduce the behavior:
1. Start HyperCache with config '...'
2. Execute command '...'
3. See error

**Expected Behavior**
What you expected to happen.

**Environment**
- OS: [e.g., Ubuntu 22.04, macOS 13.0]
- Go Version: [e.g., 1.23.2]
- HyperCache Version: [e.g., v0.1.0]

**Additional Context**
Any other context about the problem.
```

### Suggesting Features

For feature requests, please provide:

1. **Use Case**: Why is this feature needed?
2. **Proposed Solution**: How should it work?
3. **Alternatives**: Other solutions you've considered
4. **Breaking Changes**: Any compatibility concerns

### Code Contributions

#### 1. Fork and Clone

```bash
# Fork the repository on GitHub
git clone https://github.com/YOUR_USERNAME/HyperCache.git
cd HyperCache
git remote add upstream https://github.com/rishabhverma17/HyperCache.git
```

#### 2. Create a Feature Branch

```bash
# Create and switch to a new branch
git checkout -b feature/your-feature-name

# Or for bug fixes
git checkout -b fix/issue-description
```

#### 3. Make Your Changes

Follow the [Development Workflow](#development-workflow) and [Coding Standards](#coding-standards) below.

#### 4. Test Your Changes

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific tests
go test ./internal/cache/ -v

# Run benchmarks (if applicable)
go test -bench=. ./internal/cache/
```

#### 5. Commit Your Changes

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```bash
# Examples of good commit messages
git commit -m "feat: add support for Redis SCAN command"
git commit -m "fix: resolve race condition in cluster membership"
git commit -m "docs: update API documentation with new commands"
git commit -m "perf: optimize hash ring lookup performance"
git commit -m "test: add integration tests for persistence layer"
```

**Commit Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `test`: Adding or updating tests
- `perf`: Performance improvements
- `refactor`: Code refactoring
- `style`: Code style changes
- `chore`: Maintenance tasks

#### 6. Push and Create Pull Request

```bash
# Push your branch
git push origin feature/your-feature-name

# Create a Pull Request on GitHub
```

## Development Workflow

### Before Starting Development

1. **Sync with Upstream**:
```bash
git fetch upstream
git checkout main
git merge upstream/main
```

2. **Update Dependencies**:
```bash
go mod download
go mod tidy
```

### During Development

1. **Run Tests Frequently**:
```bash
# Quick tests during development
go test ./internal/cache/ -short

# Full test suite before committing
go test ./...
```

2. **Check Code Quality**:
```bash
# Format code
go fmt ./...

# Vet code
go vet ./...

# Run linter (if golangci-lint is installed)
golangci-lint run
```

3. **Update Documentation**: Keep docs in sync with code changes

## Coding Standards

### Go Code Style

1. **Follow Go Conventions**:
   - Use `gofmt` for formatting
   - Follow effective Go practices
   - Use meaningful names for variables and functions

2. **Package Organization**:
   - Keep packages focused and cohesive
   - Avoid circular dependencies
   - Use internal packages for private code

3. **Error Handling**:
   - Always handle errors explicitly
   - Use custom error types when appropriate
   - Provide context in error messages

4. **Comments and Documentation**:
   - Document all exported functions and types
   - Use complete sentences in comments
   - Explain the "why" not just the "what"

### Code Examples

#### Good Function Documentation
```go
// Set stores a key-value pair in the cache with an optional TTL.
// If ttl is 0, the key will not expire.
// Returns an error if the key cannot be stored due to memory limits.
func (c *Cache) Set(key string, value []byte, ttl time.Duration) error {
    if len(key) == 0 {
        return ErrInvalidKey
    }
    // Implementation...
}
```

#### Error Handling
```go
// Good: Explicit error handling with context
result, err := cache.Get(key)
if err != nil {
    return fmt.Errorf("failed to get key %q: %w", key, err)
}

// Bad: Ignoring errors
result, _ := cache.Get(key)
```

#### Testing
```go
func TestCacheSet(t *testing.T) {
    tests := []struct {
        name    string
        key     string
        value   []byte
        ttl     time.Duration
        wantErr bool
    }{
        {"valid key", "test", []byte("value"), 0, false},
        {"empty key", "", []byte("value"), 0, true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            cache := NewCache()
            err := cache.Set(tt.key, tt.value, tt.ttl)
            if (err != nil) != tt.wantErr {
                t.Errorf("Set() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

## Pull Request Guidelines

### Before Submitting

1. **Rebase on Latest Main**:
```bash
git fetch upstream
git rebase upstream/main
```

2. **Run Full Test Suite**:
```bash
go test ./...
go test -race ./...
```

3. **Check Build**:
```bash
go build ./...
```

### Pull Request Description

Use this template for your PR description:

```markdown
## Summary
Brief description of changes

## Changes
- List of specific changes made
- Any breaking changes
- New features added

## Testing
- [ ] All tests pass
- [ ] New tests added for new functionality
- [ ] Manual testing performed

## Documentation
- [ ] Code comments updated
- [ ] Documentation updated (if needed)
- [ ] Configuration examples updated (if needed)

## Related Issues
Fixes #123
Relates to #456
```

### Review Process

1. **Automated Checks**: CI will run tests and checks
2. **Code Review**: Maintainers will review your code
3. **Address Feedback**: Make requested changes
4. **Final Approval**: Once approved, your PR will be merged

## Types of Contributions

### 1. Bug Fixes
- Small, focused changes
- Include test cases
- Update documentation if needed

### 2. New Features
- Discuss in an issue first for large features
- Follow existing patterns and conventions
- Include comprehensive tests
- Update configuration examples

### 3. Performance Improvements
- Include benchmarks showing improvement
- Ensure no functionality regression
- Document any trade-offs

### 4. Documentation
- Fix typos and improve clarity
- Add examples and use cases
- Keep documentation in sync with code

### 5. Testing
- Add missing test coverage
- Improve existing tests
- Add integration or benchmark tests

## Release Process

HyperCache follows semantic versioning (SemVer):

- **Major** (x.0.0): Breaking changes
- **Minor** (x.y.0): New features, backwards compatible
- **Patch** (x.y.z): Bug fixes, backwards compatible

## Community Guidelines

### Code of Conduct

We are committed to providing a welcoming and inclusive environment. Please:

1. **Be Respectful**: Treat everyone with respect and kindness
2. **Be Constructive**: Provide helpful feedback and suggestions
3. **Be Patient**: Remember that everyone has different experience levels
4. **Be Open**: Welcome newcomers and help them learn

### Getting Help

- **GitHub Issues**: For bugs and feature requests
- **GitHub Discussions**: For questions and general discussion
- **Code Review**: Ask questions during the review process

## Acknowledgments

Contributors will be:
- Listed in the project's contributors list
- Mentioned in release notes (for significant contributions)
- Credited in documentation (for major features)

## Advanced Contributions

### Architecture Changes

For significant architectural changes:

1. **Create an RFC**: Document the proposal thoroughly
2. **Discuss Early**: Get feedback before implementation
3. **Prototype**: Build a minimal proof of concept
4. **Gradual Implementation**: Break into smaller PRs

### Performance Critical Code

For performance-sensitive areas:

1. **Benchmark First**: Establish baseline performance
2. **Profile Changes**: Use Go's profiling tools
3. **Document Trade-offs**: Explain performance vs. complexity
4. **Test Thoroughly**: Ensure correctness isn't compromised

### Security Considerations

For security-related changes:

1. **Follow Security Best Practices**: Use secure coding patterns
2. **Consider Attack Vectors**: Think about potential vulnerabilities
3. **Document Security Implications**: Explain security benefits/risks
4. **Test Edge Cases**: Include security-focused test cases

Thank you for contributing to HyperCache! 🚀
