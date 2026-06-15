// Package embedder selects and constructs the embedding backend cks uses for
// semantic retrieval, behind a single provider-agnostic entry point.
//
// The query path depends on ckv's types.Embedder interface, not on a concrete
// provider, so adding a new backend (a different local server, a hosted API)
// is a new case in Open plus a config value — no changes in the composer,
// the ckv adapter, or the MCP layer. Open also returns a Capability so
// cks.ops.health can advertise exactly what this instance embeds with.
package embedder

import (
	"fmt"

	"github.com/0xmhha/code-knowledge-vector/pkg/embed/ollama"
	ckvtypes "github.com/0xmhha/code-knowledge-vector/pkg/types"
)

// DefaultProvider is used when CKVConfig.Provider is empty.
const DefaultProvider = "ollama"

// Capability describes the embedding backend an instance serves. It is filled
// from config even when Open fails (Dim stays 0), so health can report the
// intended model/endpoint while signaling the backend is down.
type Capability struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	Dim      int    `json:"dim,omitempty"`
}

// knownDims pins the expected vector dimension for models whose mismatch the
// index identity check cannot catch (a provider can serve a different-
// dimensioned model under a known name). Unknown models skip the check and
// rely on ckv.Open's manifest identity match. Extend this map when adding a
// model whose dimension should be asserted.
var knownDims = map[string]int{"bge-m3": 1024}

// Open constructs an embedder for the given provider (default "ollama"),
// returning the live embedder plus a Capability describing it. On failure it
// returns a Capability populated from the requested provider/model/endpoint
// (Dim 0) so callers can still report identity in a degraded state.
func Open(provider, model, endpoint string) (ckvtypes.Embedder, Capability, error) {
	if provider == "" {
		provider = DefaultProvider
	}
	cap := Capability{Provider: provider, Model: model, Endpoint: endpoint}

	switch provider {
	case "ollama":
		if model == "" {
			model = "bge-m3"
			cap.Model = model
		}
		adapter, err := ollama.Open(ollama.Options{Endpoint: endpoint, ModelName: model})
		if err != nil {
			return nil, cap, err
		}
		dim := adapter.Dimension()
		if want, ok := knownDims[model]; ok && dim != want {
			_ = adapter.Close()
			return nil, cap, fmt.Errorf("embedder %q dim=%d, want %d", model, dim, want)
		}
		cap.Dim = dim
		return adapter, cap, nil
	default:
		return nil, cap, fmt.Errorf("unknown embedder provider %q (supported: ollama)", provider)
	}
}
