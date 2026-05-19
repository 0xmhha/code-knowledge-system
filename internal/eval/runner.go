package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	mcpgoclient "github.com/mark3labs/mcp-go/client"
	mcpgotransport "github.com/mark3labs/mcp-go/client/transport"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// toolGetForTask is the cks-mcp tool the runner invokes. Matches the
// const in internal/mcp/server.go — duplicated here to avoid a
// dependency on internal/mcp from the eval package.
const toolGetForTask = "cks.context.get_for_task"

// mcpClient is the seam over the upstream mcp-go *client.Client.
// Production code injects mcpgoclient.NewClient; tests inject a mock
// so the runner exercises full pipeline logic without a cks-mcp
// subprocess.
type mcpClient interface {
	Initialize(ctx context.Context, req mcpgo.InitializeRequest) (*mcpgo.InitializeResult, error)
	CallTool(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)
	Close() error
}

// Compile-time guarantee mcp-go's *client.Client satisfies our seam.
var _ mcpClient = (*mcpgoclient.Client)(nil)

// Metrics are the per-scenario retrieval metrics. Scalar fields are
// the median across runs (when Scenario.Runs > 1) or the single
// per-run value (when Runs == 1). LatencyMS_* fields expose the
// raw distribution: P50 == LatencyMS for 1 run, otherwise the
// nearest-rank percentile across runs. P95/Max are useful only
// at Runs >= 5 and Runs >= 2 respectively; smaller N degrades
// gracefully (P95 of 1 sample == that sample).
type Metrics struct {
	FilePrecision    float64 `json:"file_precision"`
	FileRecall       float64 `json:"file_recall"`
	FileF1           float64 `json:"file_f1"`
	TokenUtilization float64 `json:"token_utilization"`
	CitationCount    int     `json:"citation_count"`
	BodyCount        int     `json:"body_count"`
	RedactionCount   int     `json:"redaction_count"`
	LatencyMS        int64   `json:"latency_ms"` // median (legacy)
	LatencyMSP50     int64   `json:"latency_ms_p50"`
	LatencyMSP95     int64   `json:"latency_ms_p95"`
	LatencyMSMax     int64   `json:"latency_ms_max"`
}

// ScenarioResult is the per-scenario row in the final report.
type ScenarioResult struct {
	Name      string  `json:"name"`
	Prompt    string  `json:"prompt"`
	Intent    string  `json:"intent,omitempty"`
	Runs      int     `json:"runs"`
	MatchMode string  `json:"match_mode"`
	Metrics   Metrics `json:"metrics"`
	// Error, when non-empty, indicates the tool produced an error on
	// at least one run. Per-run errors are aggregated into a single
	// summary string; metric fields are best-effort over the
	// surviving runs (zero when all runs failed).
	Error string `json:"error,omitempty"`
}

// Runner owns one cks-mcp connection and executes scenarios against
// it. Not safe for concurrent Execute calls without external
// serialization — cks-mcp processes calls sequentially.
type Runner struct {
	client mcpClient
	closed bool
}

// RunnerOpts configures NewRunner.
type RunnerOpts struct {
	CKSMCPBinary string
	CKSMCPConfig string
	Env          []string
}

// NewRunner spawns cks-mcp via stdio and performs the MCP initialize
// handshake.
func NewRunner(ctx context.Context, opts RunnerOpts) (*Runner, error) {
	bin := opts.CKSMCPBinary
	if bin == "" {
		bin = "cks-mcp"
	}
	args := make([]string, 0, 2)
	if opts.CKSMCPConfig != "" {
		args = append(args, "-config", opts.CKSMCPConfig)
	}
	tp := mcpgotransport.NewStdio(bin, opts.Env, args...)
	c := mcpgoclient.NewClient(tp)
	if err := c.Start(ctx); err != nil {
		return nil, fmt.Errorf("eval: start cks-mcp: %w", err)
	}
	return newRunnerWithClient(ctx, c)
}

func newRunnerWithClient(ctx context.Context, c mcpClient) (*Runner, error) {
	req := mcpgo.InitializeRequest{}
	req.Params.ProtocolVersion = mcpgo.LATEST_PROTOCOL_VERSION
	req.Params.ClientInfo = mcpgo.Implementation{
		Name:    "cks-eval",
		Version: "0.0.1",
	}
	if _, err := c.Initialize(ctx, req); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("eval: initialize: %w", err)
	}
	return &Runner{client: c}, nil
}

// Close shuts down the underlying mcp-go client. Idempotent.
func (r *Runner) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	return r.client.Close()
}

// Execute runs s.Runs invocations of cks.context.get_for_task against
// the prompt, collects per-run metrics, and folds them into a median
// ScenarioResult.
//
// Errors from individual runs are recorded on the result rather than
// returned — a scenario with one bad run still appears in the report.
// Only structural failures (nil scenario, broken transport on every
// run) return an error.
func (r *Runner) Execute(ctx context.Context, s *Scenario) (*ScenarioResult, error) {
	if s == nil {
		return nil, errors.New("eval: nil scenario")
	}
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("eval: scenario invalid: %w", err)
	}

	perRun := make([]Metrics, 0, s.Runs)
	errMsgs := make([]string, 0)

	for i := 0; i < s.Runs; i++ {
		m, runErr := r.executeOnce(ctx, s)
		if runErr != nil {
			errMsgs = append(errMsgs, fmt.Sprintf("run %d: %v", i+1, runErr))
			continue
		}
		perRun = append(perRun, m)
	}

	out := &ScenarioResult{
		Name:      s.Name,
		Prompt:    s.Prompt,
		Intent:    string(s.Intent),
		Runs:      s.Runs,
		MatchMode: string(s.MatchMode),
		Metrics:   medianMetrics(perRun),
	}
	if len(errMsgs) > 0 {
		out.Error = strings.Join(errMsgs, "; ")
	}
	return out, nil
}

// executeOnce performs one tool call + metric computation.
func (r *Runner) executeOnce(ctx context.Context, s *Scenario) (Metrics, error) {
	req := mcpgo.CallToolRequest{}
	req.Params.Name = toolGetForTask
	req.Params.Arguments = map[string]any{"prompt": s.Prompt}

	t0 := time.Now()
	res, err := r.client.CallTool(ctx, req)
	elapsed := time.Since(t0)
	if err != nil {
		return Metrics{}, fmt.Errorf("CallTool: %w", err)
	}
	if res != nil && res.IsError {
		return Metrics{}, fmt.Errorf("%s", concatText(res))
	}

	pack, err := decodePack(res)
	if err != nil {
		return Metrics{}, fmt.Errorf("decode: %w", err)
	}

	p, rec, f := precisionRecall(s.ExpectedCitations, pack.Citations, s.MatchMode)
	return Metrics{
		FilePrecision:    p,
		FileRecall:       rec,
		FileF1:           f,
		TokenUtilization: pack.Metadata.UtilizationRatio,
		CitationCount:    len(pack.Citations),
		BodyCount:        len(pack.Bodies),
		RedactionCount:   len(pack.SanitizeReport),
		LatencyMS:        elapsed.Milliseconds(),
	}, nil
}

// medianMetrics folds per-run Metrics into one. Scalar fields and
// counts use the median (cks is deterministic so median ~= run-1).
// Latency gets the percentile treatment because operationally it's
// the field most likely to vary across runs:
//
//   LatencyMS    = p50 (legacy field name kept for backwards compat)
//   LatencyMSP50 = p50 (explicit alias)
//   LatencyMSP95 = p95 (== p50 when len < 5)
//   LatencyMSMax = p100 (max observed)
func medianMetrics(ms []Metrics) Metrics {
	if len(ms) == 0 {
		return Metrics{}
	}
	prec := make([]float64, len(ms))
	rec := make([]float64, len(ms))
	f1 := make([]float64, len(ms))
	tok := make([]float64, len(ms))
	cit := make([]float64, len(ms))
	bod := make([]float64, len(ms))
	red := make([]float64, len(ms))
	lat := make([]float64, len(ms))
	for i, m := range ms {
		prec[i] = m.FilePrecision
		rec[i] = m.FileRecall
		f1[i] = m.FileF1
		tok[i] = m.TokenUtilization
		cit[i] = float64(m.CitationCount)
		bod[i] = float64(m.BodyCount)
		red[i] = float64(m.RedactionCount)
		lat[i] = float64(m.LatencyMS)
	}
	p50 := int64(percentile(lat, 0.5))
	return Metrics{
		FilePrecision:    median(prec),
		FileRecall:       median(rec),
		FileF1:           median(f1),
		TokenUtilization: median(tok),
		CitationCount:    int(median(cit)),
		BodyCount:        int(median(bod)),
		RedactionCount:   int(median(red)),
		LatencyMS:        p50,
		LatencyMSP50:     p50,
		LatencyMSP95:     int64(percentile(lat, 0.95)),
		LatencyMSMax:     int64(percentile(lat, 1.0)),
	}
}

// decodePack mirrors cks-agent's decodePack: prefer StructuredContent,
// fall back to text content JSON. Duplicated rather than shared to
// keep internal/eval free of cmd/cks-agent imports.
func decodePack(res *mcpgo.CallToolResult) (contract.EvidencePack, error) {
	if res == nil {
		return contract.EvidencePack{}, errors.New("nil tool result")
	}
	if res.StructuredContent != nil {
		raw, err := json.Marshal(res.StructuredContent)
		if err != nil {
			return contract.EvidencePack{}, fmt.Errorf("marshal structured: %w", err)
		}
		var p contract.EvidencePack
		if err := json.Unmarshal(raw, &p); err != nil {
			return contract.EvidencePack{}, fmt.Errorf("decode structured pack: %w", err)
		}
		return p, nil
	}
	txt := concatText(res)
	if txt == "" {
		return contract.EvidencePack{}, errors.New("empty tool result")
	}
	var p contract.EvidencePack
	if err := json.Unmarshal([]byte(txt), &p); err != nil {
		return contract.EvidencePack{}, fmt.Errorf("decode text pack: %w", err)
	}
	return p, nil
}

func concatText(res *mcpgo.CallToolResult) string {
	if res == nil {
		return ""
	}
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcpgo.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}
