# Pull Request

## Summary

<!-- What does this PR do? One paragraph is enough. Link the issue it closes if applicable.
     Closes #<issue number> -->

## Type of change

- [ ] Bug fix (non-breaking change that fixes an issue)
- [ ] New feature (non-breaking change that adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to change)
- [ ] Documentation / tests only

## Changes made

<!-- Bullet-point the key files / functions changed and why -->

-
-

## Testing

<!-- How did you verify this works? Include test commands you ran. -->

```bash
CGO_ENABLED=1 go test ./...
node --test web/static/js/*.test.cjs
go vet ./...
```

## Checklist

- [ ] `go build ./...` passes with no warnings
- [ ] `go vet ./...` passes clean
- [ ] `CGO_ENABLED=1 go test ./...` passes (or document why specific tests are skipped)
- [ ] `node --test web/static/js/*.test.cjs` passes (run from project root)
- [ ] `npm run build` in `web/` produces no errors
- [ ] New packages include at least a smoke test
- [ ] `CHANGELOG.md` updated under `[Unreleased]`
- [ ] No exploitation, C2, credential-dumping, or WebShell tooling introduced
- [ ] No new dependencies added without justification in this description

## Screenshots (if UI change)

<!-- Before / after screenshots for any visible UI change -->
