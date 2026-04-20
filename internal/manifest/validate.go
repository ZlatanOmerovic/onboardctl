package manifest

import (
	_ "embed"
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"
)

//go:embed assets/schema.json
var bundledSchema []byte

// Lint validates a YAML manifest or extras file against the bundled JSON Schema.
//
// If path is empty, Lint targets the user's default extras file (see
// DefaultExtrasPath). A missing default file is a specific error so the CLI
// can tell the user where to put it, rather than silently succeeding.
func Lint(path string) error {
	if path == "" {
		path = DefaultExtrasPath()
		if path == "" {
			return errors.New("cannot resolve default extras path (no XDG_CONFIG_HOME or HOME)")
		}
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("no file at %s — pass a path to lint something else", path)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	// yaml.v3 decodes maps as map[string]any (good for JSON Schema) except
	// when the YAML uses non-string keys, which we don't support in our
	// manifest. If users write non-string keys, the validator will reject
	// the resulting shape.
	var doc any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse yaml %s: %w", path, err)
	}
	doc = normaliseYAMLForJSON(doc)

	c := jsonschema.NewCompiler()
	if err := c.AddResource("manifest.schema.json", bytes.NewReader(bundledSchema)); err != nil {
		return fmt.Errorf("register bundled schema: %w", err)
	}
	sch, err := c.Compile("manifest.schema.json")
	if err != nil {
		return fmt.Errorf("compile bundled schema: %w", err)
	}

	if err := sch.Validate(doc); err != nil {
		var ve *jsonschema.ValidationError
		if errors.As(err, &ve) {
			return fmt.Errorf("manifest %s is invalid:\n%s", path, formatValidationError(ve))
		}
		return fmt.Errorf("validate %s: %w", path, err)
	}
	return nil
}

// normaliseYAMLForJSON walks a decoded YAML tree and rewrites any
// map[interface{}]interface{} (which yaml.v2 produced) into map[string]any
// so a JSON-Schema validator can consume it. yaml.v3 already uses
// map[string]any for most inputs; this is belt-and-braces.
func normaliseYAMLForJSON(v any) any {
	switch t := v.(type) {
	case map[any]any:
		m := make(map[string]any, len(t))
		for k, vv := range t {
			m[fmt.Sprint(k)] = normaliseYAMLForJSON(vv)
		}
		return m
	case map[string]any:
		for k, vv := range t {
			t[k] = normaliseYAMLForJSON(vv)
		}
		return t
	case []any:
		for i, vv := range t {
			t[i] = normaliseYAMLForJSON(vv)
		}
		return t
	default:
		return v
	}
}

// formatValidationError renders a JSON Schema validation error in a way
// that tells the user where the problem is and what was expected.
func formatValidationError(ve *jsonschema.ValidationError) string {
	var b strings.Builder
	walkValidationError(ve, &b, 0)
	return b.String()
}

func walkValidationError(ve *jsonschema.ValidationError, b *strings.Builder, depth int) {
	if ve.InstanceLocation != "" || ve.Message != "" {
		fmt.Fprintf(b, "%s%s: %s\n", strings.Repeat("  ", depth), ve.InstanceLocation, ve.Message)
	}
	for _, c := range ve.Causes {
		walkValidationError(c, b, depth+1)
	}
}
