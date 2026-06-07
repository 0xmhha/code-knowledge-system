// Command cks-promotion-worksheet emits a domain-expert review
// worksheet for every entry in a project that matches the requested
// status (default: needs_verification). One markdown section per entry
// + one shared header + one shared footer.
//
// The worksheet is the input artifact for the substantive
// (human-checked) half of STATUS_LIFECYCLE.md §verification-checklist.
// Mechanical checks are assumed already-passing (cks-inventory-check
// returns 0 errors); the expert only fills in the four substantive
// questions per entry and a decision triple (APPROVE / REVISE / REJECT).
//
// What the generator pre-fills (best-effort heuristic; reviewer can
// override):
//   - "Maps to:" cross-reference into the 07 §9 hallucination-risk
//     catalog and the 08 §4 T2 trap catalog. Score = token overlap of
//     entry.title + invariants + pitfalls against each catalog item's
//     keyword set. The top match shows; the reviewer corrects if wrong.
//   - "suggested: <file:line>" hint on each invariant slot. Score =
//     token overlap of the invariant text against each CodeAnchor's
//     reason. No hint shown if every anchor scores zero.
//
// What the generator does NOT pre-fill:
//   - Pitfall failure modes (Q4). Expert writes them; they are the
//     single best evidence that the pitfall is real and not decorative.
//   - The decision. Expert decides.
//
// Usage:
//
//	cks-promotion-worksheet -project docs/domain-knowledge/projects/go-stablenet
//	cks-promotion-worksheet -project ... -out verification-worksheet.md
//	cks-promotion-worksheet -project ... -status draft        # any status filter
//	cks-promotion-worksheet -project ... -priority P0         # priority filter
//
// Exit codes:
//   - 0: worksheet emitted (even if 0 entries matched — that is a
//     legitimate empty-queue case, not a tool error).
//   - 2: usage error (missing -project, project directory unreadable).
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/0xmhha/code-knowledge-system/internal/inventory"
)

func main() {
	var (
		projectDir = flag.String("project", "", "project directory (contains project.yaml, subsystems.yaml, entries/)")
		status     = flag.String("status", "needs_verification", "entry status to include")
		priority   = flag.String("priority", "", "optional priority filter (P0|P1|P2|P3); empty = all")
		outPath    = flag.String("out", "", "output path; empty = stdout")
	)
	flag.Parse()

	if *projectDir == "" {
		fmt.Fprintln(os.Stderr, "cks-promotion-worksheet: -project is required")
		flag.Usage()
		os.Exit(2)
	}

	p, err := inventory.LoadProject(*projectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cks-promotion-worksheet: %v\n", err)
		os.Exit(2)
	}

	entries := filterAndSort(p, *status, *priority)

	var w io.Writer = os.Stdout
	if *outPath != "" {
		f, err := os.Create(*outPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cks-promotion-worksheet: %v\n", err)
			os.Exit(2)
		}
		defer f.Close()
		w = f
	}

	renderHeader(w, p, *status, *priority, len(entries))
	for _, e := range entries {
		renderEntry(w, p, e)
	}
	renderFooter(w, p)

	fmt.Fprintf(os.Stderr, "cks-promotion-worksheet: %d entries emitted (status=%s priority=%s)\n",
		len(entries), *status, displayOrAll(*priority))
}

func displayOrAll(s string) string {
	if s == "" {
		return "(all)"
	}
	return s
}

func filterAndSort(p *inventory.Project, status, priority string) []inventory.Entry {
	out := make([]inventory.Entry, 0, len(p.Entries))
	for _, id := range p.EntryIDsSorted() {
		e := p.Entries[id]
		if e.Status != status {
			continue
		}
		if priority != "" && e.Priority != priority {
			continue
		}
		out = append(out, e)
	}
	return out
}

// ------------- byzantine catalog (07 §9) ----------------

// catalogItem is one row from 07 §9. The keyword set is what the
// heuristic matches against an entry's title + invariants + pitfalls.
type catalogItem struct {
	num      int
	label    string
	keywords []string
}

// catalog mirrors 07 §9 in coding-agent/docs/r1-refactor/. Keep in sync
// when 07 §9 changes — this is a hard-coded mirror by design (avoids a
// cross-repo doc lookup at runtime).
var catalog = []catalogItem{
	{1, "stake-weighted voting / slashing",
		[]string{"stake", "weight", "weighted", "slashing", "slash", "equal", "power", "validator"}},
	{2, "reorg / probabilistic-finality (forker, Td inert under WBFT)",
		[]string{"reorg", "finality", "forker", "totaldifficulty", "probabilistic", "td", "fork"}},
	{3, "ETH-denominated assumptions (WKRC, base-fee redistribution)",
		[]string{"eth", "wkrc", "basefee", "base-fee", "burn", "redistribut", "ether", "wei", "denominat"}},
	{4, "quorum reimplementation (ceil(N−F) split-brain)",
		[]string{"quorum", "ceil", "n-f", "split-brain", "supermajority", "count"}},
	{5, "feepayer sigHash payload",
		[]string{"feepayer", "sighash", "payload", "fee-delegation", "fee_delegation", "sign", "rlp"}},
	{6, "missing blacklist enforcement point",
		[]string{"blacklist", "transfer", "blacklisted", "blocked", "denylist"}},
	{7, "concurrency (Core.current off mutex, txpool mutation)",
		[]string{"concurrenc", "mutex", "rwmutex", "lock", "race", "txpool", "goroutine"}},
	{8, "breaking cherry-pick-ability (interleave StableNet into geth)",
		[]string{"cherry", "cherrypick", "cherry-pick", "upstream", "isolate", "interleave", "geth-origin"}},
}

func mapToCatalog(e inventory.Entry) (string, int) {
	hay := strings.ToLower(e.Title + " " + e.Summary + " " +
		strings.Join(e.Invariants, " ") + " " + strings.Join(e.Pitfalls, " ") + " " + e.ID)
	best := catalogItem{num: 0}
	bestScore := 0
	for _, c := range catalog {
		score := 0
		for _, kw := range c.keywords {
			if strings.Contains(hay, kw) {
				score++
			}
		}
		if score > bestScore {
			best = c
			bestScore = score
		}
	}
	if bestScore == 0 {
		return "(no automatic catalog match — reviewer to assign)", 0
	}
	return fmt.Sprintf("**07 §9 item %d** (%s)", best.num, best.label), bestScore
}

// ------------- invariant → anchor hint ----------------

// suggestedAnchor returns the file:line of the anchor whose `reason`
// text shares the most lowercased word tokens with the invariant text.
// Returns "" if no anchor has any overlap, in which case the worksheet
// shows an "implicit / structural" hint instead of a positive cite.
func suggestedAnchor(inv string, anchors []inventory.CodeAnchor) string {
	bestIdx, bestScore := -1, 0
	invToks := tokenize(inv)
	for i, a := range anchors {
		score := 0
		reasonToks := tokenize(a.Reason)
		for tok := range invToks {
			if reasonToks[tok] {
				score++
			}
		}
		if score > bestScore {
			bestIdx, bestScore = i, score
		}
	}
	if bestIdx < 0 {
		return ""
	}
	a := anchors[bestIdx]
	cite := a.File
	if a.Line > 0 {
		cite = fmt.Sprintf("%s:%d", a.File, a.Line)
	}
	if a.Symbol != "" {
		cite = fmt.Sprintf("%s `%s`", cite, a.Symbol)
	}
	return cite
}

// tokenize lowercases and splits on non-alphanumeric, dropping stop
// words that are too generic to be diagnostic ("the", "a", etc.).
func tokenize(s string) map[string]bool {
	out := map[string]bool{}
	var cur strings.Builder
	flush := func() {
		w := cur.String()
		cur.Reset()
		if len(w) < 3 {
			return
		}
		if stopWord[w] {
			return
		}
		out[w] = true
	}
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
}

var stopWord = map[string]bool{
	"the": true, "and": true, "for": true, "not": true, "are": true,
	"every": true, "with": true, "from": true, "into": true, "over": true,
	"any": true, "all": true, "via": true, "this": true,
	"that": true, "set": true, "use": true, "uses": true, "used": true,
	"per": true, "out": true, "off": true, "but": true, "has": true,
	"have": true, "had": true, "did": true, "does": true,
	"when": true, "where": true, "which": true, "what": true, "who": true,
	"how": true, "its": true, "their": true, "them": true, "they": true,
	"one": true, "two": true, "may": true, "can": true, "must": true,
	"will": true, "would": true, "could": true, "should": true,
	"only": true, "also": true, "than": true, "then": true, "even": true,
	"some": true, "each": true, "such": true, "very": true, "more": true,
	"most": true, "many": true, "much": true, "other": true, "another": true,
}

// ------------- rendering ----------------

func renderHeader(w io.Writer, p *inventory.Project, status, priority string, n int) {
	fmt.Fprintf(w, "# Verification Worksheet — `%s`\n\n", p.ID)
	fmt.Fprintf(w, "> **Generated**: %s by `cks-promotion-worksheet`\n",
		time.Now().UTC().Format("2006-01-02"))
	fmt.Fprintf(w, "> **Filter**: status=`%s`, priority=`%s`\n",
		status, displayOrAll(priority))
	fmt.Fprintf(w, "> **Entries in queue**: %d\n\n", n)

	fmt.Fprintln(w, "## Session header (one per session, fill once)")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "- **Project**: `%s` @ `<commit-sha>`\n", p.ID)
	fmt.Fprintln(w, "- **Session date**: ____________")
	fmt.Fprintln(w, "- **Domain expert**: ____________ · **Operator**: ____________")
	fmt.Fprintln(w, "- **Pre-flight**: all anchors are file+line+symbol consistent (`cks-inventory-check` passes 0 errors).")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "### Reference catalogs (keep open in another tab)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "- Top hallucination risks: `coding-agent/docs/r1-refactor/07-domain-knowledge-curation.md` §9")
	fmt.Fprintln(w, "- T2 trap catalog: `coding-agent/docs/r1-refactor/08-p0c-foundations-t2-and-internalization.md` §4")
	fmt.Fprintln(w, "- Hallucination-risk re-statement: same file, §9")
	fmt.Fprintln(w, "- Status policy: `cks/docs/domain-knowledge/shared/STATUS_LIFECYCLE.md` §verification-checklist")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "### Decision dictionary")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "- **APPROVE** → entry promoted to `verified`; run the per-entry promotion command at the bottom of the section.")
	fmt.Fprintln(w, "- **REVISE** → return to author with the reviewer notes filled in; entry stays `needs_verification`.")
	fmt.Fprintln(w, "- **REJECT** → archive entry (rare; only if the trap is wrong, not just imprecise).")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "---")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Entries")
	fmt.Fprintln(w)
}

func renderEntry(w io.Writer, p *inventory.Project, e inventory.Entry) {
	fmt.Fprintf(w, "### `%s` · `[%s · %s · %s · risk:%s]`\n\n",
		e.ID, e.Priority, e.Subsystem, e.KnowledgeType, dashIfEmpty(e.RiskLevel))
	fmt.Fprintf(w, "**Title**: %s\n", e.Title)
	fmt.Fprintf(w, "**Source of truth**: `%s` · **Status**: `%s`\n",
		dashIfEmpty(e.SourceOfTruth), e.Status)
	catalogStr, _ := mapToCatalog(e)
	fmt.Fprintf(w, "**Maps to**: %s\n\n", catalogStr)

	fmt.Fprintln(w, "**Summary** (read first):")
	fmt.Fprintln(w)
	fmt.Fprintln(w, renderQuote(e.Summary))
	fmt.Fprintln(w)

	if len(e.CodeAnchors) > 0 {
		fmt.Fprintln(w, "**Anchors** (mechanical ✅ — already file+line+symbol consistent):")
		fmt.Fprintln(w)
		for i, a := range e.CodeAnchors {
			fmt.Fprintf(w, "%d. %s\n", i+1, formatAnchor(a))
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, "---")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "#### Substantive review — STATUS_LIFECYCLE §verification-checklist")
	fmt.Fprintln(w)

	// Q1
	fmt.Fprintln(w, "**[Q1] Is the `summary` factually correct against current code?**")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "- [ ] ✅ correct")
	fmt.Fprintln(w, "- [ ] ⚠️ partially (note what to revise)")
	fmt.Fprintln(w, "- [ ] ❌ wrong (explain)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "> reviewer note:")
	fmt.Fprintln(w)

	// Q2
	fmt.Fprintln(w, "**[Q2] Do the `code_keywords` match identifiers actually used in source?**")
	fmt.Fprintln(w)
	if len(e.CodeKeywords) > 0 {
		fmt.Fprintf(w, "Keywords: %s\n\n", renderInlineList(e.CodeKeywords))
	} else {
		fmt.Fprintln(w, "Keywords: *(none defined; consider adding)*")
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "- [ ] ✅ all match")
	fmt.Fprintln(w, "- [ ] ⚠️ partial (list which are stale)")
	fmt.Fprintln(w, "- [ ] ❌ all stale")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "> reviewer note:")
	fmt.Fprintln(w)

	// Q3 — invariants with anchor hints
	if len(e.Invariants) > 0 {
		fmt.Fprintln(w, "**[Q3] Invariants — does each one actually hold, and which code line enforces it?**")
		fmt.Fprintln(w)
		for i, inv := range e.Invariants {
			fmt.Fprintf(w, "- **I%d.** *%s*\n", i+1, inv)
			hint := suggestedAnchor(inv, e.CodeAnchors)
			if hint != "" {
				fmt.Fprintf(w, "  - [ ] ✅ enforced at `__________________` (suggested: %s)\n", hint)
			} else {
				fmt.Fprintln(w, "  - [ ] ✅ enforced at `__________________`")
			}
			fmt.Fprintln(w, "  - [ ] ⚠️ implicit / no single line (e.g. structural — absence of a field)")
			fmt.Fprintln(w, "  - [ ] ❌ not enforced")
		}
		fmt.Fprintln(w)
	}

	// Q4 — pitfalls
	if len(e.Pitfalls) > 0 {
		fmt.Fprintln(w, "**[Q4] Pitfalls — describe one concrete failure mode for each. If you can't, the pitfall is decorative.**")
		fmt.Fprintln(w)
		for i, pf := range e.Pitfalls {
			fmt.Fprintf(w, "- **P%d.** *%s*\n", i+1, pf)
			fmt.Fprintln(w, "  > expected failure mode:")
		}
		fmt.Fprintln(w)
	}

	// Q5 — procedure_steps (B6 entries)
	if len(e.ProcedureSteps) > 0 {
		fmt.Fprintln(w, "**[Q5] Procedure steps — does each step still match current tooling/code paths?**")
		fmt.Fprintln(w)
		for i, st := range e.ProcedureSteps {
			fmt.Fprintf(w, "- **S%d.** *%s*\n", i+1, st)
			fmt.Fprintln(w, "  - [ ] ✅ still accurate · [ ] ⚠️ needs revision · [ ] ❌ obsolete")
		}
		fmt.Fprintln(w)
	}

	// Q6 — constants (B7 entries)
	if len(e.Constants) > 0 {
		fmt.Fprintln(w, "**[Q6] Constants — do the values match what's in source today?**")
		fmt.Fprintln(w)
		for _, c := range e.Constants {
			cite := c.SourceFile
			if cite == "" {
				cite = "(no source_file recorded)"
			}
			fmt.Fprintf(w, "- **%s** = `%v` %s — cite: `%s`\n",
				c.Name, c.Value, c.Unit, cite)
			fmt.Fprintln(w, "  - [ ] ✅ matches · [ ] ⚠️ drifted (record new value) · [ ] ❌ removed")
		}
		fmt.Fprintln(w)
	}

	// Decision
	fmt.Fprintln(w, "**Decision**:")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "- [ ] **APPROVE** → run:")
	fmt.Fprintln(w, "  ```bash")
	fmt.Fprintf(w, "  go run ./cmd/cks-entry-verify \\\n")
	fmt.Fprintf(w, "      -project %s \\\n", relativeProjectPath(p.Dir))
	fmt.Fprintf(w, "      -entry   %s \\\n", e.ID)
	fmt.Fprintln(w, "      -by      <handle>")
	fmt.Fprintln(w, "  ```")
	fmt.Fprintln(w, "- [ ] **REVISE** → write notes above; entry stays `needs_verification`.")
	fmt.Fprintln(w, "- [ ] **REJECT** → archive entry; open a follow-up to delete or supersede.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "---")
	fmt.Fprintln(w)
}

func renderFooter(w io.Writer, p *inventory.Project) {
	fmt.Fprintln(w, "## Footer — post-session sync (runs once after all APPROVE entries are promoted)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "```bash")
	fmt.Fprintln(w, "# Re-emit policy (ckg governs) + glossary (ckv vocab) for downstream consumers")
	fmt.Fprintf(w, "./bin/cks-domain-sync   -entries %s\n", relativeProjectPath(p.Dir))
	fmt.Fprintf(w, "./bin/cks-glossary-gen  -project %s -status verified\n", relativeProjectPath(p.Dir))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "# Refresh ckv (channel ② docs) + ckg (governance edges) in lockstep.")
	fmt.Fprintln(w, "# Operator's MCP client: cks.ops.index { mode: \"full\" }")
	fmt.Fprintln(w, "```")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Notes on generator behaviour")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "- The `Maps to:` line uses a token-overlap heuristic against the 07 §9 catalog. If the suggested item is wrong, the reviewer should correct it in-line — the worksheet is treated as a working document, not as authoritative metadata.")
	fmt.Fprintln(w, "- The `suggested: …` hint on each invariant is the anchor whose `reason` field shares the most words with the invariant text. A hint is shown only when at least one anchor scores > 0; otherwise the slot is left blank.")
	fmt.Fprintln(w, "- B6 entries (`procedure_steps`) and B7 entries (`constants`) get extra question blocks (Q5/Q6); other knowledge types omit them.")
}

// ------------- formatting helpers ----------------

func dashIfEmpty(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func formatAnchor(a inventory.CodeAnchor) string {
	out := fmt.Sprintf("`%s", a.File)
	if a.Line > 0 {
		out = fmt.Sprintf("`%s:%d", a.File, a.Line)
	}
	out += "`"
	if a.Symbol != "" {
		out += fmt.Sprintf(" · `%s`", a.Symbol)
	}
	if a.Reason != "" {
		out += fmt.Sprintf(" — %s", a.Reason)
	}
	return out
}

func renderQuote(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i, l := range lines {
		lines[i] = "> " + strings.TrimSpace(l)
	}
	return strings.Join(lines, "\n")
}

func renderInlineList(items []string) string {
	q := make([]string, len(items))
	for i, it := range items {
		q[i] = "`" + it + "`"
	}
	sort.Strings(q)
	return strings.Join(q, " · ")
}

// relativeProjectPath shortens an absolute project dir to its
// docs/domain-knowledge/... tail so the inline command is readable.
// Falls back to the absolute path if the tail is not present.
func relativeProjectPath(abs string) string {
	const marker = "docs/domain-knowledge/"
	if i := strings.Index(abs, marker); i >= 0 {
		return abs[i:]
	}
	return abs
}
