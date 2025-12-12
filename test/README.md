# Test Suite

Comprehensive testing suite for the Tiny URL Shortener.

## Structure

- **`integration/`** - Integration tests for services
- **`e2e/`** - End-to-end user journey tests
- **`fixtures/`** - Test data and fixtures

## Running Tests

```bash
# Run all unit tests
go test ./...

# Run integration tests
go test ./test/integration/...

# Run e2e tests
go test ./test/e2e/...

# Run with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Test Categories

### Unit Tests
Located alongside source code in each package.

### Integration Tests
Test interaction between multiple components.

### E2E Tests
Test complete user workflows from API to database.

## Writing Tests

See [Testing Guidelines](../docs/development/testing.md)
