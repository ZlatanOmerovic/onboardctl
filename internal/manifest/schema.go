// Package manifest holds the onboardctl data model: profiles, bundles,
// items, providers, repos, and input definitions.
//
// Two documents feed into a live Manifest:
//
//  1. The bundled default at internal/manifest/assets/default.yaml
//     (curated, ships with the binary).
//  2. Optional user extras at ~/.config/onboardctl/extras.yaml
//     (free-form; validated against the bundled JSON Schema).
//
// Load performs the merge; Lint validates extras against the schema.
package manifest

// SchemaVersion is the current top-level version of the manifest format.
// It is distinct from the onboardctl binary version — only data-model
// breaking changes bump this.
const SchemaVersion = 1

// Manifest is the top-level document.
//
// All named maps use the logical ID as the key (e.g. "vlc", "base-system",
// "fullstack-web"). This makes merges trivial: extras.yaml simply adds more
// keys or overrides existing ones.
type Manifest struct {
	Version  int                `yaml:"version" json:"version"`
	Profiles map[string]Profile `yaml:"profiles,omitempty" json:"profiles,omitempty"`
	Bundles  map[string]Bundle  `yaml:"bundles,omitempty"  json:"bundles,omitempty"`
	Items    map[string]Item    `yaml:"items,omitempty"    json:"items,omitempty"`
	Repos    map[string]Repo    `yaml:"repos,omitempty"    json:"repos,omitempty"`
}

// Profile is a named preset that pulls in a set of bundles.
// extends lets one profile inherit another's bundle list.
type Profile struct {
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	Bundles     []string `yaml:"bundles" json:"bundles"`
	Extends     string   `yaml:"extends,omitempty" json:"extends,omitempty"`
}

// Bundle groups related items under one toggle. A single item can appear
// in multiple bundles — the resolver deduplicates.
type Bundle struct {
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	Items       []string `yaml:"items" json:"items"`
}

// Item is the smallest user-visible unit: one app to install or one
// configuration to apply. An Item has one or more Providers; the runner
// walks them in order and picks the first whose When matches.
type Item struct {
	Name        string     `yaml:"name" json:"name"`
	Description string     `yaml:"description,omitempty" json:"description,omitempty"`
	Bundle      string     `yaml:"bundle,omitempty" json:"bundle,omitempty"`
	Providers   []Provider `yaml:"providers" json:"providers"`
	When        *When      `yaml:"when,omitempty" json:"when,omitempty"`
	Input       *Input     `yaml:"input,omitempty" json:"input,omitempty"`
	PostInstall []string   `yaml:"post_install,omitempty" json:"post_install,omitempty"`
	Tags        []string   `yaml:"tags,omitempty" json:"tags,omitempty"`
}

// Provider kinds. Keep this in sync with the JSON Schema enum.
const (
	KindAPT            = "apt"
	KindFlatpak        = "flatpak"
	KindBinaryRelease  = "binary_release"
	KindComposerGlobal = "composer_global"
	KindNPMGlobal      = "npm_global"
	KindConfig         = "config"
	KindShell          = "shell"
)

// Provider describes one way of installing or applying an item on the
// current system. Different kinds use different subsets of the fields.
type Provider struct {
	Type    string            `yaml:"type" json:"type"`
	Package string            `yaml:"package,omitempty" json:"package,omitempty"` // apt
	ID      string            `yaml:"id,omitempty" json:"id,omitempty"`           // flatpak
	Repo    string            `yaml:"repo,omitempty" json:"repo,omitempty"`       // apt (named repo)
	Source  string            `yaml:"source,omitempty" json:"source,omitempty"`   // binary_release: owner/repo
	Asset   string            `yaml:"asset,omitempty" json:"asset,omitempty"`     // binary_release: asset regex
	Binary  string            `yaml:"binary,omitempty" json:"binary,omitempty"`   // binary_release: binary name in archive
	Apply   []string          `yaml:"apply,omitempty" json:"apply,omitempty"`     // config/shell: commands
	Check   string            `yaml:"check,omitempty" json:"check,omitempty"`     // override default check
	When    *When             `yaml:"when,omitempty" json:"when,omitempty"`
	Extra   map[string]string `yaml:"extra,omitempty" json:"extra,omitempty"`
}

// Repo declares a third-party apt repository used by one or more items.
// The runner materialises it into /etc/apt/keyrings and /etc/apt/sources.list.d
// the first time it's needed.
type Repo struct {
	Kind           string `yaml:"kind" json:"kind"`                           // apt
	Keyring        string `yaml:"keyring,omitempty" json:"keyring,omitempty"` // URL
	KeyringDearmor bool   `yaml:"keyring_dearmor,omitempty" json:"keyring_dearmor,omitempty"`
	Source         string `yaml:"source" json:"source"` // apt sources line ({keyring}/{codename} templated)
	When           *When  `yaml:"when,omitempty" json:"when,omitempty"`
}

// When gates whether an item, provider, or repo applies on this machine.
// All non-empty fields must match; an empty When is an unconditional yes.
type When struct {
	DistroID      []string `yaml:"distro_id,omitempty" json:"distro_id,omitempty"`
	DistroFamily  []string `yaml:"distro_family,omitempty" json:"distro_family,omitempty"`
	Codename      []string `yaml:"codename,omitempty" json:"codename,omitempty"`
	Desktop       []string `yaml:"desktop,omitempty" json:"desktop,omitempty"`
	Arch          []string `yaml:"arch,omitempty" json:"arch,omitempty"`
	PackageExists []string `yaml:"package_exists,omitempty" json:"package_exists,omitempty"`
}

// Input kinds. Keep in sync with JSON Schema.
const (
	InputChoice = "choice"
	InputText   = "text"
	InputForm   = "form"
	InputBool   = "bool"
)

// Input describes how to prompt the user before applying a config item.
// Phase 3's TUI will render each kind differently.
type Input struct {
	Kind    string   `yaml:"kind" json:"kind"`
	Prompt  string   `yaml:"prompt" json:"prompt"`
	Default any      `yaml:"default,omitempty" json:"default,omitempty"`
	Choices []string `yaml:"choices,omitempty" json:"choices,omitempty"`
	Source  string   `yaml:"source,omitempty" json:"source,omitempty"`
	Fields  []Field  `yaml:"fields,omitempty" json:"fields,omitempty"`
}

// Field is one input in a form.
type Field struct {
	Name    string `yaml:"name" json:"name"`
	Prompt  string `yaml:"prompt" json:"prompt"`
	Default string `yaml:"default,omitempty" json:"default,omitempty"`
	Secret  bool   `yaml:"secret,omitempty" json:"secret,omitempty"`
}
