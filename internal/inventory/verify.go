package inventory

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// MarkVerified rewrites the entry YAML at path so that exactly three
// fields are set:
//
//	status:           "verified"
//	last_verified_at: date     (YYYY-MM-DD)
//	verified_by:      reviewer
//
// Every other field — including ordering, comments, blank lines, and
// multi-line literal styles — is preserved because the rewrite is
// done via yaml.Node mutation on the parsed tree, not a struct-based
// re-marshal.
//
// MarkVerified refuses to add missing keys. If status / last_verified_at /
// verified_by are absent from the entry, that signals a hand-edited
// file the reviewer should look at, and quietly inventing the keys
// would obscure the deviation. Today's authored entries always include
// placeholder `null` values for last_verified_at and verified_by, so
// this check never fires in practice; it is a guard against template
// drift, not a routine code path.
func MarkVerified(path, date, reviewer string) error {
	if date == "" {
		return fmt.Errorf("inventory.MarkVerified: date is required")
	}
	if !dateYYYYMMDDPattern.MatchString(date) {
		return fmt.Errorf("inventory.MarkVerified: date %q must match YYYY-MM-DD", date)
	}
	if reviewer == "" {
		return fmt.Errorf("inventory.MarkVerified: reviewer is required")
	}

	buf, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("inventory.MarkVerified: read %q: %w", path, err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(buf, &root); err != nil {
		return fmt.Errorf("inventory.MarkVerified: parse %q: %w", path, err)
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) != 1 {
		return fmt.Errorf("inventory.MarkVerified: %q: unexpected YAML document shape", path)
	}
	mapNode := root.Content[0]
	if mapNode.Kind != yaml.MappingNode {
		return fmt.Errorf("inventory.MarkVerified: %q: top-level value is not a mapping", path)
	}

	// last_verified_at gets DoubleQuotedStyle so the value is preserved
	// as a string. yaml.v3 will otherwise emit YYYY-MM-DD as a plain
	// scalar, which YAML 1.1-style parsers (including yaml.v3 itself
	// when unmarshalling into `any`) auto-detect as a timestamp. That
	// is harmless for our own structs (LastVerifiedAt is typed string),
	// but it breaks downstream tools that walk entries via `any`. Quoting
	// makes the intent explicit at the cost of one pair of " characters.
	for _, kv := range []struct {
		key, value string
		style      yaml.Style
	}{
		{"status", "verified", 0},
		{"last_verified_at", date, yaml.DoubleQuotedStyle},
		{"verified_by", reviewer, 0},
	} {
		if err := setMapScalar(mapNode, kv.key, kv.value, kv.style); err != nil {
			return fmt.Errorf("inventory.MarkVerified: %q: %w", path, err)
		}
	}

	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return fmt.Errorf("inventory.MarkVerified: encode %q: %w", path, err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("inventory.MarkVerified: encode %q: %w", path, err)
	}

	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		return fmt.Errorf("inventory.MarkVerified: write %q: %w", path, err)
	}
	return nil
}

// setMapScalar finds key in mapping m and replaces its value with a
// string scalar carrying value, emitted in the requested style (pass 0
// for the yaml.v3 default plain style). Returns an error when key is
// missing — MarkVerified treats that as a template drift signal.
//
// The replacement strips any prior tag (e.g. !!null) so the value is
// re-emitted as a plain string regardless of the previous node's type
// (the entry's last_verified_at: null placeholder is the common case).
func setMapScalar(m *yaml.Node, key, value string, style yaml.Style) error {
	if m.Kind != yaml.MappingNode {
		return fmt.Errorf("setMapScalar: target is not a mapping")
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		k := m.Content[i]
		v := m.Content[i+1]
		if k.Kind == yaml.ScalarNode && k.Value == key {
			v.Kind = yaml.ScalarNode
			v.Tag = ""
			v.Style = style
			v.Value = value
			v.Content = nil
			return nil
		}
	}
	return fmt.Errorf("setMapScalar: key %q not present in mapping", key)
}
