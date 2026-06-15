// Package config loads and validates cks runtime configuration:
//
//   - Config:            top-level cks settings (backends, listen, logging,
//     sanitize). Loaded from a YAML file at startup.
//   - SanitizeRuleset:   the sanitize-rule catalog used by the composer's
//     final-stage redaction (see sanitize.go).
//
// The loader is permissive on optional fields and strict on dangerous ones:
// unknown logging levels, malformed regex patterns in sanitize rules, and
// unrecognized RedactionActions are rejected at load time so that bugs
// surface early rather than silently degrading the LLM-boundary defense.
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// configVersion is the only supported top-level version. Bumping requires a
// migration path documented in the changelog.
const configVersion = 1

// Config is the top-level cks settings root.
type Config struct {
	Version  int            `yaml:"version"`
	Backends BackendsConfig `yaml:"backends"`
	Listen   ListenConfig   `yaml:"listen"`
	Logging  LoggingConfig  `yaml:"logging"`
	Sanitize SanitizeConfig `yaml:"sanitize"`
	Vocab    VocabConfig    `yaml:"vocab"`
	Domain   DomainConfig   `yaml:"domain"`
}

// DomainConfig configures channel ② (domain-knowledge embedding). Empty
// ProjectDir disables it: cks.ops.index then refreshes only code + ckg.
type DomainConfig struct {
	// ProjectDir is the cks domain-knowledge project directory exported
	// before a full ckv build (e.g. docs/domain-knowledge/projects/go-stablenet).
	ProjectDir string `yaml:"project_dir"`
	// CorpusDir is the export output and the ckv --docs root
	// (e.g. generated/domain-corpus/go-stablenet).
	CorpusDir string `yaml:"corpus_dir"`
}

// VocabConfig controls the optional vocabulary resolver wired into
// composer Stage 1. The resolver expands user prompts with code keywords
// drawn from a project's curated glossary, so retrieval can match
// Korean / domain-vague terms against the identifiers actually present
// in the source tree. An empty GlossaryPath disables vocab expansion —
// Stage 1 falls back to the verbatim prompt, which is identical to the
// pre-vocab behavior.
type VocabConfig struct {
	// GlossaryPath is the YAML glossary file the vocab.Resolver loads at
	// startup. Relative paths resolve against the cks binary's cwd.
	GlossaryPath string `yaml:"glossary_path"`
}

// BackendsConfig holds connection settings for ckg and ckv.
type BackendsConfig struct {
	CKG CKGConfig `yaml:"ckg"`
	CKV CKVConfig `yaml:"ckv"`
}

// CKGConfig is the ckg client connection profile.
//
// SourceRoot points at the working tree the ckg index was built
// against. Used by composer.Stage4's FilesystemFetcher to resolve
// Citation.File paths (which are stored as repo-relative strings).
// Empty SourceRoot makes the fetcher resolve against cwd — the
// common case when cks-mcp runs from the indexed repo's root.
type CKGConfig struct {
	Path       string `yaml:"path"`
	SourceRoot string `yaml:"source_root"`
	TimeoutMS  int    `yaml:"timeout_ms"`
	// BinaryPath is the ckg binary used by the cks.ops.index maintenance tool
	// (G8 shells `ckg build`); empty disables the ckg leg of that tool. Not
	// used on the query path (ckg is opened in-process via ckgclient.NewReal).
	BinaryPath string `yaml:"binary_path"`
	// PolicyFile is the ckg governance policy (the `policies/policy.yaml`
	// emitted by cks-domain-sync). When set, cks.ops.index passes it to
	// `ckg build --policy-file` so governed_by edges are rebuilt with the
	// index; empty omits the flag (no governance enrichment).
	PolicyFile string `yaml:"policy_file"`
}

// CKVConfig is the ckv client connection profile.
//
// BinaryPath: ckv exposes its real Engine only through its own MCP binary
// (the Go-level constructors live in internal/), so the cks ckv adapter
// spawns `ckv mcp --out=<Path>` as a subprocess. BinaryPath gives the
// absolute path to that binary; empty means "look up `ckv` on $PATH".
type CKVConfig struct {
	Path string `yaml:"path"`
	// BinaryPath is retained for the agent-triggered index op (cks.ops.index,
	// G8 shells `ckv reindex`); it is NOT used on the query path anymore —
	// G1 imports pkg/ckv in-process, so there is no `ckv mcp` subprocess.
	BinaryPath string `yaml:"binary_path"`
	// TimeoutMS is unused by the in-process query path (kept for config
	// back-compat; the subprocess call timeout it bounded no longer exists).
	TimeoutMS int `yaml:"timeout_ms"`
	// EmbedModel is the Ollama model name (e.g. "bge-m3") used to construct
	// the in-process embedder. Must match the model the index was built with.
	EmbedModel string `yaml:"embed_model"`
	// OllamaURL is the Ollama daemon endpoint. Empty resolves to
	// http://localhost:11434 (and the CKV_OLLAMA_ENDPOINT env override).
	OllamaURL string `yaml:"ollama_url"`
}

// ListenConfig controls how cks exposes its surface to callers.
//
// Transport selects the MCP transport: "stdio" (default) wires one client to
// a subprocess over stdin/stdout; "http" serves Streamable HTTP on HTTPAddr,
// which lets one host run several cks instances (different DBs/models) on
// different ports, reachable by remote Claude Code clients. cks.ops.health
// advertises each instance's model + indexed commit so callers can tell them
// apart.
//
// HTTPAddr is loopback-only unless AllowRemote is set: exposing the retrieval
// surface to the network is an explicit opt-in, not the default.
type ListenConfig struct {
	HTTPAddr string `yaml:"http_addr"`
	MCPStdio bool   `yaml:"mcp_stdio"`
	// Transport is "stdio" | "http". Empty falls back to MCPStdio (stdio)
	// for config back-compat.
	Transport string `yaml:"transport"`
	// AllowRemote permits binding HTTPAddr to a non-loopback (routable)
	// address. Default false keeps the listener loopback-only. Enabling it
	// exposes retrieval to the network; the sanitize ruleset still applies,
	// but operators should add auth / network controls (token auth is a
	// follow-up).
	AllowRemote bool `yaml:"allow_remote"`
}

// ResolvedTransport returns the effective MCP transport, defaulting to stdio
// when unset (honoring the legacy MCPStdio flag).
func (l ListenConfig) ResolvedTransport() string {
	if l.Transport != "" {
		return strings.ToLower(l.Transport)
	}
	return "stdio"
}

// LoggingConfig matches the footprint package's Mode/Level vocabulary and
// adds output directories for footprint and audit logs.
type LoggingConfig struct {
	Level        string `yaml:"level"`
	Mode         string `yaml:"mode"`
	FootprintDir string `yaml:"footprint_dir"`
	AuditDir     string `yaml:"audit_dir"`
}

// SanitizeConfig points to the sanitize ruleset and sets composer-wide
// fallback behavior.
type SanitizeConfig struct {
	// RulesPath is the YAML file with the SanitizeRuleset. May be empty
	// during early development if the composer is wired with an in-memory
	// ruleset instead.
	RulesPath string `yaml:"rules_path"`
	// DefaultAction is the RedactionAction to apply when a sanitize rule
	// omits its own action. Empty means the rule must specify its action.
	DefaultAction contract.RedactionAction `yaml:"default_action"`
	// FailClosedOnUnknownRule, when true, causes the composer to fail
	// closed (refuse the pack) on any unrecognized rule action or hit
	// against a malformed rule — defense in depth against ruleset edits
	// that bypass redaction by typo.
	FailClosedOnUnknownRule bool `yaml:"fail_closed_on_unknown_rule"`
}

// Default returns the out-of-box cks configuration. Used by tests and as a
// fallback when no config file is supplied.
func Default() *Config {
	return &Config{
		Version: configVersion,
		Backends: BackendsConfig{
			CKG: CKGConfig{TimeoutMS: 5000},
			CKV: CKVConfig{TimeoutMS: 3000},
		},
		Listen: ListenConfig{
			HTTPAddr: "127.0.0.1:8080",
			MCPStdio: true,
		},
		Logging: LoggingConfig{
			Level:        "info",
			Mode:         "prod",
			FootprintDir: "./logs/footprint",
			AuditDir:     "./logs/audit",
		},
		Sanitize: SanitizeConfig{
			RulesPath:               "./policies/sanitization_rules.yaml",
			DefaultAction:           contract.RedactionDrop,
			FailClosedOnUnknownRule: true,
		},
	}
}

// Load reads path as YAML and returns a validated Config.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %q: %w", path, err)
	}
	// Expand ${VAR}/$VAR so one config file is portable across machines
	// (e.g. source_root: "${GO_STABLENET_ROOT}"); the per-machine path then
	// comes from the environment, not a committed/copied absolute path.
	return LoadBytes([]byte(os.ExpandEnv(string(data))))
}

// LoadBytes parses raw YAML bytes into a validated Config. Useful for tests
// and for callers that source configuration from non-file media (embed,
// remote KV, etc.).
func LoadBytes(data []byte) (*Config, error) {
	var c Config
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true) // reject typos / unknown keys to surface bugs early
	if err := dec.Decode(&c); err != nil {
		if errors.Is(err, ErrEmptyDocument) {
			return nil, err
		}
		return nil, fmt.Errorf("config: decode: %w", err)
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// Validate reports the first structural problem found in c. Acts as the
// gate that lets a Config through to runtime use.
func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("config: nil")
	}
	if c.Version != configVersion {
		return fmt.Errorf("config: version=%d, want %d", c.Version, configVersion)
	}

	switch strings.ToLower(c.Logging.Level) {
	case "", "debug", "info", "warn", "warning", "error":
	default:
		return fmt.Errorf("config: logging.level=%q invalid (debug|info|warn|error)", c.Logging.Level)
	}
	switch strings.ToLower(c.Logging.Mode) {
	case "", "prod", "dev":
	default:
		return fmt.Errorf("config: logging.mode=%q invalid (prod|dev)", c.Logging.Mode)
	}

	switch strings.ToLower(c.Listen.Transport) {
	case "", "stdio", "http":
	default:
		return fmt.Errorf("config: listen.transport=%q invalid (stdio|http)", c.Listen.Transport)
	}
	if strings.ToLower(c.Listen.Transport) == "http" && c.Listen.HTTPAddr == "" {
		return fmt.Errorf("config: listen.transport=http requires listen.http_addr")
	}
	// Loopback is enforced by default; AllowRemote is the explicit opt-in to
	// bind a routable address for remote callers.
	if c.Listen.HTTPAddr != "" && !c.Listen.AllowRemote {
		if err := validateLoopback(c.Listen.HTTPAddr); err != nil {
			return fmt.Errorf("config: listen.http_addr: %w (set listen.allow_remote: true to bind a routable address)", err)
		}
	}

	switch c.Sanitize.DefaultAction {
	case "", contract.RedactionMask, contract.RedactionDrop, contract.RedactionFailClosed:
	default:
		return fmt.Errorf("config: sanitize.default_action=%q invalid", c.Sanitize.DefaultAction)
	}

	if (c.Domain.ProjectDir == "") != (c.Domain.CorpusDir == "") {
		return fmt.Errorf("config: domain.project_dir and domain.corpus_dir must both be set or both empty")
	}

	return nil
}

// validateLoopback returns nil only when addr is a host:port pointing at a
// loopback address (127.0.0.0/8, ::1). Non-loopback bindings are rejected
// because cks should not expose its surface to remote callers in Phase 0/1.
func validateLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid host:port %q: %w", addr, err)
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("host %q is not an IP and not 'localhost'; non-loopback bindings are rejected", host)
	}
	if !ip.IsLoopback() {
		return fmt.Errorf("host %q is not a loopback address", host)
	}
	return nil
}

// ErrEmptyDocument is returned by LoadBytes when the input contains no YAML
// document at all (distinct from a syntactically valid empty mapping).
var ErrEmptyDocument = errors.New("config: empty YAML document")
