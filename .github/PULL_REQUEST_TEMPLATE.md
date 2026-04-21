## Description

Brief description of the changes in this PR.

## Related Issue

Closes #(issue number)

## Type of Change

- [ ] 🐛 Bug fix (non-breaking change that fixes an issue)
- [ ] ✨ New feature (non-breaking change that adds functionality)
- [ ] 💥 Breaking change (fix or feature that would cause existing functionality to change)
- [ ] 📝 Documentation update
- [ ] ♻️ Refactoring (no functional changes)
- [ ] 🧪 Test improvement

## Layer Affected (if applicable)

- [ ] CLI (`cmd/claude`, `internal/bootstrap`)
- [ ] TUI (`internal/tui`, `internal/commands`)
- [ ] Tools (`internal/tools`, `internal/permissions`)
- [ ] Engine (`internal/engine`, `internal/coordinator`)
- [ ] Services (`internal/api`, `internal/oauth`, `internal/mcp`)
- [ ] Infra (`pkg/types`, `internal/config`, `internal/state`)
- [ ] N/A

## Checklist

- [ ] My code follows the [Go coding standards](CONTRIBUTING.md#coding-standards) of this project
- [ ] I have run `gofmt` and `goimports` on my code
- [ ] I have added tests that prove my fix/feature works
- [ ] All new and existing tests pass (`make test`)
- [ ] I have updated documentation where necessary
- [ ] My changes generate no new warnings or lint errors (`make lint`)
- [ ] I have added comments to exported functions/types

## Testing

Describe the tests you ran and how to reproduce them.

```bash
# Example
make test
make vet
make lint
```

## Screenshots (if applicable)

Add screenshots to help explain your changes (especially for TUI changes).
