# Contributing

Thanks for your interest. `onboardctl` is early-stage; the core data model and
CLI surface are still settling. Issues and design discussions are more useful
than code for now.

## Local development

```bash
make build   # compile
make test    # unit tests
make vet     # go vet
make fmt     # gofmt -w
```

Golangci-lint is optional but recommended:

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
make lint
```

## Conventions

- Conventional Commits for messages (`feat:`, `fix:`, `docs:`, `chore:`, etc.).
- Any new provider kind (beyond the initial apt / flatpak / binary_release / config / shell /
  composer_global / npm_global) requires a matching JSON-Schema enum bump and a pass through `make lint`.
- Keep the bundled manifest small and opinionated; niche tools belong in user `extras.yaml`.
