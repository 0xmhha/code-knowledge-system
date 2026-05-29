package inventory

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// projectFile is the subset of project.yaml the loader needs. Unknown
// fields are tolerated so additions to project.yaml (indexing config,
// authoritative_docs, etc.) don't break the loader. KnownFields=false
// is the default for yaml.v3 — no extra wiring needed.
type projectFile struct {
	ID            string `yaml:"id"`
	Name          string `yaml:"name"`
	CodeRoot      string `yaml:"code_root"`
	SchemaVersion int    `yaml:"schema_version"`
}

// LoadProject reads a project directory and returns its in-memory view.
// dir is the project directory itself (e.g.
// docs/domain-knowledge/projects/go-stablenet), not the parent.
//
// All three sources are required:
//   - <dir>/project.yaml
//   - <dir>/subsystems.yaml
//   - <dir>/entries/*.yaml (at least one file)
//
// Failure to read or parse any of them is a hard error; the validator
// is meant to be run by the same hands that author the inventory, so a
// missing file is a bug worth reporting up front.
func LoadProject(dir string) (*Project, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("inventory: abs %q: %w", dir, err)
	}

	pf, err := loadProjectYAML(filepath.Join(absDir, "project.yaml"))
	if err != nil {
		return nil, err
	}

	subsystems, order, err := loadSubsystems(filepath.Join(absDir, "subsystems.yaml"))
	if err != nil {
		return nil, err
	}

	entries, err := loadEntries(filepath.Join(absDir, "entries"))
	if err != nil {
		return nil, err
	}

	codeRoot := pf.CodeRoot
	if codeRoot != "" && !filepath.IsAbs(codeRoot) {
		// project.yaml's code_root is documented as an absolute path
		// (see go-stablenet's project.yaml). Treat a relative value
		// as relative to the project directory — that lets test
		// fixtures use sibling trees without hard-coding /Users paths.
		codeRoot = filepath.Join(absDir, codeRoot)
	}

	return &Project{
		Dir:            absDir,
		ID:             pf.ID,
		Name:           pf.Name,
		CodeRoot:       codeRoot,
		SchemaVersion:  pf.SchemaVersion,
		Subsystems:     subsystems,
		SubsystemOrder: order,
		Entries:        entries,
	}, nil
}

func loadProjectYAML(path string) (*projectFile, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("inventory: read %q: %w", path, err)
	}
	var pf projectFile
	if err := yaml.Unmarshal(buf, &pf); err != nil {
		return nil, fmt.Errorf("inventory: parse %q: %w", path, err)
	}
	if pf.ID == "" {
		return nil, fmt.Errorf("inventory: %q missing required field id", path)
	}
	return &pf, nil
}

func loadSubsystems(path string) (map[string]Subsystem, []string, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("inventory: read %q: %w", path, err)
	}
	var list []Subsystem
	if err := yaml.Unmarshal(buf, &list); err != nil {
		return nil, nil, fmt.Errorf("inventory: parse %q: %w", path, err)
	}
	if len(list) == 0 {
		return nil, nil, fmt.Errorf("inventory: %q declares no subsystems", path)
	}
	out := make(map[string]Subsystem, len(list))
	order := make([]string, 0, len(list))
	for _, s := range list {
		if s.ID == "" {
			return nil, nil, fmt.Errorf("inventory: %q has subsystem with empty id", path)
		}
		if _, dup := out[s.ID]; dup {
			return nil, nil, fmt.Errorf("inventory: %q has duplicate subsystem id %q", path, s.ID)
		}
		out[s.ID] = s
		order = append(order, s.ID)
	}
	return out, order, nil
}

func loadEntries(dir string) (map[string]Entry, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, fmt.Errorf("inventory: glob %q: %w", dir, err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("inventory: no entries found at %q", dir)
	}
	sort.Strings(files)

	out := make(map[string]Entry, len(files))
	for _, f := range files {
		e, err := LoadEntry(f)
		if err != nil {
			return nil, err
		}
		if _, dup := out[e.ID]; dup {
			return nil, fmt.Errorf("inventory: duplicate entry id %q (second seen at %s)", e.ID, f)
		}
		out[e.ID] = e
	}
	return out, nil
}

// LoadEntry reads a single entry YAML file and returns the parsed Entry.
// SourcePath is set to the absolute path of the input file so downstream
// validators and the verify CLI can report and rewrite it.
//
// LoadEntry does not validate the entry against the project — that is
// ValidateProject's job. The only check here is "the file parses as
// YAML and has the basic shape of an entry" (a non-empty id field).
// Status, cross-references, anchor existence: all left to the validator.
func LoadEntry(path string) (Entry, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, fmt.Errorf("inventory: read %q: %w", path, err)
	}
	var e Entry
	if err := yaml.Unmarshal(buf, &e); err != nil {
		return Entry{}, fmt.Errorf("inventory: parse %q: %w", path, err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return Entry{}, fmt.Errorf("inventory: abs %q: %w", path, err)
	}
	e.SourcePath = absPath
	if e.ID == "" {
		return Entry{}, fmt.Errorf("inventory: %q missing required field id", path)
	}
	return e, nil
}
