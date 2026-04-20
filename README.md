# onboardctl

> Interactive, profile-driven post-install provisioner for Debian-based Linux.

`onboardctl` turns a fresh Debian / Ubuntu / Mint / Pop!_OS / Elementary / MX / Kali install
into a finished workstation in one sitting — interactively. Pick a terminal, shell, and prompt; pick a
profile (essentials, fullstack-web, devops, polyglot-dev, everything); drill into bundles and toggle
individual items; re-run any time to see a diff of what changed rather than a reinstall.

> **Status:** early development. Phase 1 (data model, detection, `status`, `lint`) is usable now;
> installers and the TUI arrive in Phases 2 and 3. See [Roadmap](#roadmap).

## What it is (and isn't)

**It is**

- An opinionated but *skippable* provisioning wizard — every step has a "keep current" option.
- A thin layer on top of `apt`, `flatpak`, GitHub releases, `composer global`, and a few system-config commands.
- Extensible: ship your own additions via `~/.config/onboardctl/extras.yaml`, validated against the bundled JSON Schema.
- Stateful: tracks what it installed and when, so re-runs are diffs rather than full reinstalls.
- **Snap-averse on Ubuntu:** pins `snapd` to never be auto-installed, swaps snap-gated apps (Firefox/Chromium/Thunderbird) for real `.deb`s or Flatpaks.

**It isn't**

- An uninstaller. `onboardctl` only adds; removing things is the user's responsibility.
- Multi-distro beyond the Debian family. Fedora, Arch, openSUSE, etc. are out of scope by design.
- A dotfiles manager. Use [chezmoi](https://chezmoi.io) or [yadm](https://yadm.io) for that.
- A replacement for Ansible/Nix. This is for humans on laptops, not fleets in production.

## Architecture

Three layers, each small and replaceable:

```
┌────────────────────────────────────────────────────────────────┐
│  TUI (Phase 3: Bubble Tea)                                     │
│  — init wizard: terminal / shell / prompt / theme              │
│  — profile wizard: profile → bundles → per-item toggles        │
└────────────────────────────────────────────────────────────────┘
                 ↓ emits selected items
┌────────────────────────────────────────────────────────────────┐
│  Runner (Phase 2)                                              │
│  — resolves profile → bundles → items → providers              │
│  — dispatches to provider.Registry                             │
│  — writes ~/.config/onboardctl/state.yaml                      │
└────────────────────────────────────────────────────────────────┘
                 ↓ calls
┌────────────────────────────────────────────────────────────────┐
│  Providers (Phase 2: one per kind)                             │
│  apt · flatpak · binary_release · composer_global              │
│  npm_global · config · shell                                   │
└────────────────────────────────────────────────────────────────┘

        Source of truth: internal/manifest/assets/default.yaml
                       + ~/.config/onboardctl/extras.yaml (optional)
        Validated by:   internal/manifest/assets/schema.json
```

Providers implement a three-method interface (`Kind`, `Check`, `Install`) and register into a central `Registry`.
That's what makes every phase independent: the TUI knows nothing about apt; the runner knows nothing about Bubble Tea;
providers know nothing about profiles.

## Subcommands

| Command                  | Status    | What it does                                            |
|--------------------------|-----------|---------------------------------------------------------|
| `onboardctl status`      | ✅ Phase 1 | Print detected env + loaded-manifest summary            |
| `onboardctl lint [path]` | ✅ Phase 1 | Validate a YAML manifest against the bundled JSON Schema |
| `onboardctl version`     | ✅ Phase 1 | Print version, commit, build date                        |
| `onboardctl install`     | ✅ Phase 2 | Headless install by item / bundle / profile             |
| `onboardctl profile`     | ✅ Phase 3 | Interactive profile picker (TUI)                         |
| `onboardctl init`        | ⏳ Phase 3+ | TUI wizard: terminal / shell / prompt / theme           |
| `onboardctl export`      | ⏳ Phase 4 | Emit current state as a shareable extras YAML           |

## Quickstart

**Install (users):**

```bash
curl -fsSL https://raw.githubusercontent.com/ZlatanOmerovic/onboardctl/main/scripts/install.sh | bash
```

Installs to `~/.local/bin/onboardctl`. No sudo. Options:
`--install-dir PATH` · `--version vX.Y.Z` · `--help`. The installer
pulls the latest GitHub release matching your arch (amd64 / arm64)
and verifies SHA-256 when a checksum file is present.

**Build from source (developers):**

```bash
git clone https://github.com/ZlatanOmerovic/onboardctl.git
cd onboardctl
make build
./onboardctl status
./onboardctl lint path/to/your-extras.yaml
```

Requires Go 1.24+.

## Extending the manifest

Create `~/.config/onboardctl/extras.yaml`. Anything you define there is merged on top of the bundled manifest
— extras wins on key collisions. Example:

```yaml
version: 1

items:
  spotify:
    name: Spotify
    description: Music streaming.
    bundle: media
    providers:
      - type: flatpak
        id: com.spotify.Client

bundles:
  media:
    name: Media
    description: Media apps.
    items: [vlc, spotify]  # reuse the bundled vlc item + add spotify
```

Validate before committing:

```bash
onboardctl lint ~/.config/onboardctl/extras.yaml
```

## Roadmap

| Phase                | Scope                                                                                                 |
|----------------------|-------------------------------------------------------------------------------------------------------|
| **1 — foundation** ✅ | Data model; `status` / `lint`; distro + desktop detection                                             |
| **2 — installers** ✅ | `apt` / `shell` / `config` / `binary_release` / `composer_global` providers; `install` subcommand; state file; When gates; apt repo bootstrap |
| **3 — TUI (MVP)** ✅  | Bubble Tea `profile` picker with Catppuccin Mocha palette; item counts per profile   |
| 3+ — TUI follow-ups   | Per-item toggles; config-input forms (timezone, git identity); four status markers on items; live install progress |
| 4 — release          | `curl \| sh` bootstrap, GitHub Actions releases, screenshots, Homebrew tap (maybe), Debian `.deb`     |

## Non-goals (and the tools that cover them)

| You want to…                                   | Use…                                                      |
|------------------------------------------------|-----------------------------------------------------------|
| Declare a fleet of workstations declaratively  | [Ansible](https://www.ansible.com), [Nix home-manager](https://nix-community.github.io/home-manager/) |
| Manage dotfiles                                | [chezmoi](https://chezmoi.io), [yadm](https://yadm.io)    |
| Install on Arch / Fedora                       | [omakub-multidistro](https://github.com/con5ole/omakub_multidistro), [omakase-blue](https://github.com/foundation-devices/omakase-blue) |
| Uninstall what onboardctl added                | `sudo apt purge …`, `flatpak uninstall …`, or write one yourself |

## License

MIT. See [LICENSE](./LICENSE).
