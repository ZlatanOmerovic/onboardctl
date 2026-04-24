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

## [0.2.0] — 2026-04-24

Second release. Adds uninstall, parallelism, version pinning, post-install
hooks, theme coverage across four more desktops, an upgrade command, and
two new installer helpers (shell completions + man pages).

### Added

#### Subcommands
- `rollback` — undo the last non-dry-run install in LIFO order via each
  provider's new `Uninstall` method. Dry-run by default; `--yes` applies.
- `upgrade` — download the latest release for the current GOOS/GOARCH,
  verify SHA-256 against the published checksums.txt, and atomically
  replace the running binary. `--check` previews, `--version` pins.
- `completion install [--shell bash|zsh|fish]` — generate and install the
  completion script to XDG / system paths. Auto-detects shell from
  `$SHELL`; `--stdout` prints to stdout.
- `manpage install` — render and install onboardctl man pages. `--system`
  writes to `/usr/share/man/man1/` (needs sudo); default is user-local.
  `--stdout` prints the top-level page.

#### Runner
- **Parallel install.** Apt / shell / config remain serial (system locks,
  user ordering). Flatpak / npm_global / composer_global / binary_release
  run concurrently through a bounded pool (size 4). Race-tested.
- **Rollback / transaction.** Each run records an `installed` list in
  state; `install --rollback-on-failure` undoes the run's successful
  installs if any subsequent item fails. Provider implementations gain
  an optional `Uninstaller` interface — wired on apt (`apt-get purge`),
  flatpak, npm_global, composer_global, and binary_release.
- **Offline mode.** `install --offline` refuses items whose provider
  would touch the network (apt, flatpak, binary_release, npm_global,
  composer_global); config / shell items still run. Repo bootstrap is
  skipped.

#### Manifest
- **Version pinning.** Optional `version:` field on apt / npm_global /
  composer_global providers. Apt maps to `pkg=version`; npm to `pkg@version`;
  composer to `pkg:version`.
- **Post-install hooks.** Optional `post_install:` list of shell commands
  on each item; runs after a successful Install, before state is saved.
  Failure aborts the item.
- **Four more desktop themes.** KDE (Breeze / Breeze Dark via
  `lookandfeeltool`), Xfce (`xfconf-query`), MATE (`gsettings org.mate`),
  Cinnamon (`gsettings org.cinnamon`). Init wizard step 4 picks the right
  item for the detected desktop.

#### Global
- Update-check against GitHub releases — cached 24h in
  `$XDG_CACHE_HOME/onboardctl/latest.json`. Surfaces as a single-line
  notice in `status`, `version`, and `doctor`. Disabled by
  `--no-update-check` or `ONBOARDCTL_NO_UPDATE_CHECK=1`. Dev builds skip
  the check.

### Notes
- `rollback` undoes only items onboardctl installed via one of the five
  uninstall-capable providers. Config / shell items are skipped with a
  log line because the runner cannot generically revert arbitrary
  commands.
- `upgrade` replaces the running binary via atomic rename in the same
  directory. If that directory isn't writable (usually `/usr/local/bin`),
  it errors out rather than failing mid-write; re-run with sudo.

## [Unreleased]
