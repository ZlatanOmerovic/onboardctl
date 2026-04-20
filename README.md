# onboardctl

> Interactive, profile-driven post-install provisioner for Debian-based Linux.

`onboardctl` turns a fresh Debian / Ubuntu / Mint / Pop!_OS / Elementary / MX / Kali / etc. install into a
finished workstation in one sitting — interactively. Pick a terminal, shell, and prompt; pick a
profile (essentials, fullstack-web, devops, polyglot-dev, everything); drill into bundles and toggle
individual items; re-run any time and see a diff of what changed.

This is an early-development repository. See the [roadmap](#roadmap) below.

## What it is (and isn't)

**It is**

- An opinionated but *skippable* provisioning wizard — every step has a "keep current" option.
- A thin layer on top of `apt`, `flatpak`, GitHub releases, `composer global`, and a few system-config commands.
- Extensible: ship your own additions via `~/.config/onboardctl/extras.yaml`, validated against the bundled JSON Schema.
- Stateful: tracks what it installed and when, so re-runs are diffs rather than full reinstalls.

**It isn't**

- An uninstaller. `onboardctl` only adds; removing things is the user's responsibility.
- Multi-distro beyond the Debian family. Fedora, Arch, openSUSE, etc. are out of scope by design.
- A dotfiles manager. Use [chezmoi](https://chezmoi.io) or [yadm](https://yadm.io) for that.

## Roadmap

| Phase | Scope |
|---|---|
| **1 — foundation** *(current)* | Data model, distro/DE detection, `status` + `lint` subcommands |
| 2 — installers | Real `apt`, `flatpak`, `binary_release`, `config` providers + headless `install` subcommand |
| 3 — TUI | Bubble Tea wizards (`init`, `profile`), per-item toggles, status markers |
| 4 — release | `curl \| sh` bootstrap, GitHub Actions releases, docs |

## Quickstart (developer)

```bash
make build      # builds ./onboardctl
./onboardctl status
./onboardctl lint path/to/extras.yaml
```

Released binaries and a proper install script will arrive in Phase 4.

## License

MIT. See [LICENSE](./LICENSE).
