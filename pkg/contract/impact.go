package contract

// ImpactCategory groups reverse-dependency hits by the kind of coupling.
type ImpactCategory string

const (
	ImpactCallers     ImpactCategory = "callers"
	ImpactInterface   ImpactCategory = "interface"
	ImpactTypeUsers   ImpactCategory = "type_users"
	ImpactDistributed ImpactCategory = "distributed"
	ImpactConcurrent  ImpactCategory = "concurrent"
	ImpactOther       ImpactCategory = "other"
)

// ImpactGroup is one category of reverse-dependency hits.
type ImpactGroup struct {
	Category ImpactCategory `json:"category"`
	Hits     []Citation     `json:"hits"`
}

// ImpactResult is the response of a change-impact analysis. It groups
// all symbols that would be affected by modifying the seed symbol,
// partitioned by the kind of coupling.
type ImpactResult struct {
	Seed   string        `json:"seed"`
	Groups []ImpactGroup `json:"groups"`
}
