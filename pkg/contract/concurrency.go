package contract

// ConcurrencyModule is one symbol in a seed's concurrency blast radius: a
// goroutine/channel/lock the seed spawns, sends to, or acquires, or a module
// that shares concurrent state with it. Backs cks.context.concurrency_impact.
type ConcurrencyModule struct {
	Citation Citation `json:"citation"`
	Qname    string   `json:"qname"`
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	// Direction is the seed-relative role of this module:
	//   affects      — the seed affects it (downstream of the seed)
	//   affected_by  — it affects the seed (upstream of the seed)
	//   both         — reached in both directions
	Direction string `json:"direction"`
}

// ConcurrencyResult is the response of a concurrency-impact analysis: every
// module within Depth hops of the seed over concurrency edges (spawns,
// sends_to, recvs_from, acquires_lock, accessed_under_lock).
type ConcurrencyResult struct {
	Seed     string              `json:"seed"`
	Depth    int                 `json:"depth"`
	Modules  []ConcurrencyModule `json:"modules,omitempty"`
	NotFound bool                `json:"not_found,omitempty"`
}
