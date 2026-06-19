// Command cks-anchor-refresh re-resolves each domain-knowledge entry's
// code_anchor line number from its symbol against the ckg graph and rewrites
// the stored line when it has drifted. This makes anchors resistant to the
// line drift that happens whenever the indexed source tree moves: the symbol
// is the stable key, the line is derived.
//
// Conservative by design:
//   - only anchors that carry BOTH a symbol and a line are considered;
//   - the resolved citation must be in the SAME file as the anchor (line drift
//     only). A symbol that no longer resolves in its recorded file (moved /
//     renamed / deleted) is reported as UNRESOLVED for human review, never
//     auto-rewritten — changing a file path or dropping an anchor is a
//     judgement call, not a mechanical refresh.
//
// Usage:
//
//	cks-anchor-refresh -project <dir> -graph <graph.db> [-check]
//
// Exit codes: 0 = clean (or fixed); 1 = drift/unresolved found (in -check mode,
// or unresolved anchors in write mode); 2 = usage / IO error.
package main

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/0xmhha/code-knowledge-system/internal/ckgclient"
	"github.com/0xmhha/code-knowledge-system/internal/inventory"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

func main() {
	projectDir := flag.String("project", "", "domain-knowledge project directory (contains project.yaml and entries/)")
	graphPath := flag.String("graph", "", "path to the ckg graph.db the anchors resolve against")
	check := flag.Bool("check", false, "report drift without writing; exit 1 if any anchor needs attention")
	maxShift := flag.Int("max-shift", 15, "only auto-apply when the line moved by at most this many lines; larger moves are reported as REVIEW (likely a call-site / sub-line anchor that points inside or near the symbol rather than at its definition, which cannot be mechanically refreshed)")
	flag.Parse()

	if *projectDir == "" || *graphPath == "" {
		fmt.Fprintln(os.Stderr, "usage: cks-anchor-refresh -project <dir> -graph <graph.db> [-check] [-max-shift N]")
		os.Exit(2)
	}

	if err := run(*projectDir, *graphPath, *check, *maxShift); err != nil {
		fmt.Fprintln(os.Stderr, "cks-anchor-refresh:", err)
		os.Exit(2)
	}
}

// lineUpdate is a single anchor whose line must change, keyed by (file, symbol)
// so the YAML rewriter can find the exact anchor inside code_anchors.
type lineUpdate struct {
	file, symbol     string
	oldLine, newLine int
}

func run(projectDir, graphPath string, check bool, maxShift int) error {
	proj, err := inventory.LoadProject(projectDir)
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	cli, err := ckgclient.NewReal(graphPath)
	if err != nil {
		return fmt.Errorf("open graph %q: %w", graphPath, err)
	}
	defer cli.Close()
	ctx := context.Background()

	// Deterministic order: entries by id.
	ids := make([]string, 0, len(proj.Entries))
	for id := range proj.Entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var checked, clean, drift, review, unresolved, locSkipped int
	updatesByPath := map[string][]lineUpdate{}

	for _, id := range ids {
		e := proj.Entries[id]
		for _, a := range e.CodeAnchors {
			if a.ResolvedKind() == inventory.AnchorKindLoc {
				// loc anchors point at an arbitrary line inside enclosing_symbol
				// by design (a call site / gate / branch); refreshing them to a
				// definition line would corrupt the author's intent. Never
				// repoint — that is the whole reason the kind exists (it makes
				// the old maxShift "REVIEW" guess explicit and deterministic).
				locSkipped++
				continue
			}
			if a.Symbol == "" || a.Line == 0 {
				continue // line is not symbol-derived; nothing to refresh
			}
			checked++
			cits, err := cli.FindSymbol(ctx, a.Symbol, ckgclient.SymbolOpts{})
			if err != nil {
				return fmt.Errorf("%s: FindSymbol %q: %w", id, a.Symbol, err)
			}
			newLine, ok := resolveInFile(cits, a.File, a.Line)
			if !ok {
				unresolved++
				fmt.Printf("UNRESOLVED %s  %s:%d  symbol %q not found in that file (moved/renamed/deleted?)\n",
					id, a.File, a.Line, a.Symbol)
				continue
			}
			if newLine == a.Line {
				clean++
				continue
			}
			shift := newLine - a.Line
			if shift < 0 {
				shift = -shift
			}
			if shift > maxShift {
				// Too far to be a definition that merely drifted: this anchor's
				// line almost certainly points at a call site or a specific
				// statement near/inside the symbol, not at the symbol's
				// definition. Refreshing it to the definition line would corrupt
				// the author's intent, so flag it for a human instead.
				review++
				fmt.Printf("REVIEW     %s  %s  %q  line %d (symbol defined at %d; moved %d > max-shift %d - likely a call-site/sub-line anchor)\n",
					id, a.File, a.Symbol, a.Line, newLine, shift, maxShift)
				continue
			}
			drift++
			fmt.Printf("DRIFT      %s  %s  %q  line %d -> %d\n", id, a.File, a.Symbol, a.Line, newLine)
			updatesByPath[e.SourcePath] = append(updatesByPath[e.SourcePath],
				lineUpdate{file: a.File, symbol: a.Symbol, oldLine: a.Line, newLine: newLine})
		}
	}

	fmt.Printf("\nchecked=%d clean=%d drift=%d review=%d unresolved=%d loc_skipped=%d\n", checked, clean, drift, review, unresolved, locSkipped)

	if check {
		if drift > 0 || review > 0 || unresolved > 0 {
			os.Exit(1)
		}
		return nil
	}

	// Write mode: apply the line updates, preserving YAML formatting.
	files := make([]string, 0, len(updatesByPath))
	for p := range updatesByPath {
		files = append(files, p)
	}
	sort.Strings(files)
	for _, p := range files {
		if err := rewriteAnchorLines(p, updatesByPath[p]); err != nil {
			return fmt.Errorf("rewrite %s: %w", p, err)
		}
		fmt.Printf("updated %s (%d anchor(s))\n", p, len(updatesByPath[p]))
	}
	if review > 0 || unresolved > 0 {
		os.Exit(1) // review/unresolved anchors need human attention even after a successful refresh
	}
	return nil
}

// resolveInFile picks the current start line for an anchor from the citations
// FindSymbol returned, restricted to the anchor's file. When several citations
// share the file (overloads / same-named symbols), the one whose start line is
// closest to the recorded line wins, so a refresh never jumps to an unrelated
// same-named symbol.
func resolveInFile(cits []contract.Citation, file string, recordedLine int) (int, bool) {
	best, bestDelta, found := 0, 0, false
	for _, c := range cits {
		if c.File != file {
			continue
		}
		d := c.StartLine - recordedLine
		if d < 0 {
			d = -d
		}
		if !found || d < bestDelta {
			best, bestDelta, found = c.StartLine, d, true
		}
	}
	return best, found
}

// rewriteAnchorLines updates the `line:` of the named anchors by editing the
// raw text line-by-line, so every byte of formatting — comments, blank lines,
// ordering, quote and block styles — is preserved and a re-run produces no diff
// beyond the changed line numbers. (A yaml.Node round-trip drops blank lines.)
//
// It walks the file tracking the current anchor's file/symbol as it passes the
// `- file:` / `symbol:` lines inside the code_anchors sequence; when it reaches
// a `line:` whose enclosing anchor matches a pending update (by file, symbol,
// and old value), it rewrites just the number, keeping the line's indentation.
func rewriteAnchorLines(path string, ups []lineUpdate) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 1024*1024), 4*1024*1024)

	inAnchors := false
	curFile, curSymbol := "", ""
	applied := 0

	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " "))

		switch {
		case trimmed == "code_anchors:" || strings.HasPrefix(trimmed, "code_anchors:"):
			inAnchors = true
			curFile, curSymbol = "", ""
		case inAnchors && indent == 0 && trimmed != "" && !strings.HasPrefix(trimmed, "-"):
			// a new top-level key ends the code_anchors block
			inAnchors = false
		}

		if inAnchors {
			if k, v, ok := anchorKV(trimmed); ok {
				switch k {
				case "file":
					if strings.HasPrefix(trimmed, "- ") {
						curSymbol = "" // "- file:" starts a new anchor item
					}
					curFile = unquoteYAML(v)
				case "symbol":
					curSymbol = unquoteYAML(v)
				case "line":
					if old, perr := strconv.Atoi(strings.TrimSpace(v)); perr == nil {
						for _, u := range ups {
							if u.file == curFile && u.symbol == curSymbol && u.oldLine == old {
								line = line[:indent] + "line: " + strconv.Itoa(u.newLine)
								applied++
								break
							}
						}
					}
				}
			}
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if applied == 0 {
		return fmt.Errorf("matched no anchors to update")
	}
	// Preserve trailing-newline shape: Scanner strips the final newline; if the
	// original did not end in one, drop the one we appended.
	b := out.Bytes()
	if len(raw) > 0 && raw[len(raw)-1] != '\n' && len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	return os.WriteFile(path, b, 0o644)
}

// anchorKV parses a "key: value" (or "- key: value") YAML scalar line, returning
// the key and the raw value. Returns ok=false for non-scalar lines.
func anchorKV(trimmed string) (key, value string, ok bool) {
	s := strings.TrimPrefix(trimmed, "- ")
	i := strings.IndexByte(s, ':')
	if i <= 0 {
		return "", "", false
	}
	return s[:i], strings.TrimSpace(s[i+1:]), true
}

func unquoteYAML(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
		return v[1 : len(v)-1]
	}
	return v
}
