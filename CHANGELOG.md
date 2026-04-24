# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] — 2026-04-20

First release. The tool has enough surface to turn a fresh Debian-family
install into a working developer workstation.

### Added

#### Subcommands
- `status` — detected env + manifest summary; `--plan PROFILE` for a
  non-interactive per-item plan dump.
- `lint` — validate extras.yaml against the bundled JSON Schema.
- `install` — headless install by `--profile` / `--bundle` / `--items`
  / `--from-export`; `--dry-run`, `--yes`, `--skip`, `--swap-drift`.
- `profile` — interactive TUI: picker → review with ✓/●/⚠/∅ markers →
  config-input forms (form/text/choice/bool) → dispatch, with optional
  live-progress TUI under `--apply`.
- `init` — four-step first-boot wizard (terminal / shell / prompt / theme),
  re-run-aware with install-state markers and `--skip-installed`.
- `export` — YAML or list format; `install --from-export` replays.
- `gc` — prune stale state.yaml entries not in the manifest.
- `forget` — remove specific entries or `--all`.
- `doctor` — eight environment sanity checks (distro, required tools,
  state, bundled manifest, user extras, flatpak, sudo, binary-on-PATH).
- `version` — ldflags-injected version / commit / build date.

#### Providers (all implementing Check + Install)
- `apt` with snap-drift detection (installed-via-snap surfaces as ⚠).
- `flatpak` with Flathub auto-bootstrap and scope=user/system.
- `binary_release` — GitHub release asset → tar.gz → /usr/local/bin.
- `composer_global` — for laravel/installer and friends.
- `npm_global` — for yarn / @vue/cli / etc.
- `config` — refuses items with Input unless runner supplies values.
- `shell` — escape hatch for vendor .debs and install scripts.

#### Manifest
- 5 profiles: essentials / fullstack-web / devops / polyglot-dev / everything.
- 14 bundles, 66 items, 6 apt repos (Sury, NodeSource, GitHub CLI,
  Kubernetes, AnyDesk, Microsoft VS Code).
- JSON Schema (draft-07) validates every extras.yaml.
- Repo bootstrap recognises user-managed sources and skips duplicates.
- When-gates cover distro ID / family / codename / desktop / arch.

#### Global
- Persistent `--verbose` and `--no-color` flags; NO_COLOR env honoured.
- State-file migration path (versioned, registry-based; no migrations
  registered yet but the hook is live).

#### Release infrastructure
- GoReleaser workflow on tag push produces linux/amd64 + linux/arm64
  binaries with shell completions and manpages bundled.
- `curl | sh` installer at scripts/install.sh handles latest / pinned
  versions, arch detection, SHA-256 verification.
- CI: build + test + vet + gofmt + golangci-lint + a debian:trixie
  integration container on every push.

### Notes
- `profile` subcommand is interactive (TUI); dry-run by default. Pass
  `--apply` with sudo to actually mutate the system.
- The tool only *adds* — it doesn't uninstall, except when
  `--swap-drift` removes the snap alternative of an apt-preferred item.

## [Unreleased]
