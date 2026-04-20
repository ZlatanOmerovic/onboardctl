package cli

import (
	"errors"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// parseExportFile loads a file produced by `onboardctl export` and
// returns the item IDs. It handles both formats:
//
//   - YAML: the "yaml" format has a "items:" key with a string list.
//   - list: plain one-ID-per-line; '#' starts a comment and blank lines
//     are ignored.
//
// We sniff the format: if the first non-comment line starts with "version:"
// or looks like YAML ("items:"), we parse YAML; otherwise we treat it as
// a plain list.
func parseExportFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if looksLikeExportYAML(data) {
		return parseExportYAML(data)
	}
	return parseExportList(data), nil
}

func looksLikeExportYAML(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		// First real line: is it YAML-looking?
		return strings.HasPrefix(trim, "version:") || strings.HasPrefix(trim, "items:")
	}
	return false
}

type exportDoc struct {
	Version int      `yaml:"version"`
	Items   []string `yaml:"items"`
}

func parseExportYAML(data []byte) ([]string, error) {
	var doc exportDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if len(doc.Items) == 0 {
		return nil, errors.New("export file has no 'items:' list")
	}
	return doc.Items, nil
}

func parseExportList(data []byte) []string {
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}
