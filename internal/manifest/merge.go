package manifest

// Merge returns a new Manifest containing the union of base and extras.
// On key collisions, the extras entry wins (shallow override — fields inside
// an Item, Bundle, Profile, or Repo are not deep-merged).
//
// Merge never mutates either input.
func Merge(base, extras *Manifest) *Manifest {
	switch {
	case base == nil && extras == nil:
		return &Manifest{Version: SchemaVersion}
	case base == nil:
		return cloneManifest(extras)
	case extras == nil:
		return cloneManifest(base)
	}

	out := cloneManifest(base)
	if extras.Version != 0 {
		out.Version = extras.Version
	}
	for k, v := range extras.Profiles {
		out.Profiles[k] = v
	}
	for k, v := range extras.Bundles {
		out.Bundles[k] = v
	}
	for k, v := range extras.Items {
		out.Items[k] = v
	}
	for k, v := range extras.Repos {
		out.Repos[k] = v
	}
	return out
}

func cloneManifest(m *Manifest) *Manifest {
	if m == nil {
		return nil
	}
	return &Manifest{
		Version:  m.Version,
		Profiles: copyMap(m.Profiles),
		Bundles:  copyMap(m.Bundles),
		Items:    copyMap(m.Items),
		Repos:    copyMap(m.Repos),
	}
}

func copyMap[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
