#!/usr/bin/env python3
"""Deterministic scorer for the CKG 4-way eval.

Reads results.json (workflow output: {count, results:[...]}) and the per-question
bundles/<id>/meta.json (token counts), then computes per-(question,mode):
  - location_hit: any cited file:line overlaps a ground-truth range
  - hallucinations: cited file:line that does not exist / is out of range
  - tokens: context tokens supplied (alpha/beta/delta from meta.json; gamma from retrieved_tokens)
Aggregates by mode and writes scored.json + prints a summary the report uses.
"""
import json, os, sys, math
from collections import defaultdict

ROOT = os.path.dirname(os.path.abspath(__file__))
REPO = "/Users/wm-it-25_0220/Work/github/go-stablenet"
BUNDLES = os.path.join(ROOT, "bundles")

# id -> (domain, difficulty), from dataset.yaml (kept in sync manually).
META = {
    "Q01": ("consensus-wbft", "easy"), "Q02": ("consensus-wbft", "easy"),
    "Q03": ("consensus-wbft", "easy"), "Q04": ("consensus-wbft", "medium"),
    "Q05": ("consensus-wbft", "medium"), "Q06": ("consensus-wbft", "hard"),
    "Q07": ("consensus-wbft", "easy"), "Q08": ("consensus-wbft", "medium"),
    "Q09": ("governance-validators", "easy"), "Q10": ("governance-validators", "medium"),
    "Q11": ("governance-validators", "medium"), "Q12": ("governance-validators", "medium"),
    "Q13": ("state-transition", "hard"), "Q14": ("blacklist", "easy"),
    "Q15": ("blacklist", "hard"), "Q16": ("bls-crypto", "easy"),
    "Q17": ("bls-crypto", "medium"), "Q18": ("system-contracts", "medium"),
    "Q19": ("system-contracts", "easy"), "Q20": ("genesis", "medium"),
    "Q21": ("genesis", "hard"), "Q22": ("txpool", "medium"),
    "Q23": ("txpool", "medium"), "Q24": ("evm-core", "medium"),
    "Q25": ("evm-core", "hard"), "Q26": ("p2p", "hard"),
    "Q27": ("p2p", "medium"), "Q28": ("consensus-wbft", "hard"),
    "Q29": ("governance-validators", "easy"), "Q30": ("state-transition", "medium"),
}

def norm(p):
    p = p.strip()
    # strip an accidental absolute prefix to repo (do this BEFORE touching dots,
    # and only strip a leading "./" prefix — never lstrip(".") which would mangle
    # a real ".claude/..." path into "claude/...").
    if p.startswith(REPO):
        p = p[len(REPO):]
    while p.startswith("/"):
        p = p[1:]
    if p.startswith("./"):
        p = p[2:]
    return p

def overlap(a0, a1, b0, b1):
    return a0 <= b1 and b0 <= a1

_LINECOUNT = {}
def file_line_count(relpath):
    fp = os.path.join(REPO, relpath)
    if fp in _LINECOUNT:
        return _LINECOUNT[fp]
    if not os.path.isfile(fp):
        _LINECOUNT[fp] = None
        return None
    n = 0
    with open(fp, "rb") as f:
        for _ in f:
            n += 1
    _LINECOUNT[fp] = n
    return n

# basename -> [repo-relative paths], so a citation given as a bare filename
# (no directory) can still be resolved to the real file(s) it names.
_BASENAME = {}
def build_basename_index():
    if _BASENAME:
        return
    skip = {".git", "vendor", "node_modules"}
    for dp, dns, fns in os.walk(REPO):
        dns[:] = [d for d in dns if d not in skip]
        for fn in fns:
            rel = os.path.relpath(os.path.join(dp, fn), REPO)
            _BASENAME.setdefault(fn, []).append(rel)

def resolve_candidates(relp):
    """Return repo-relative paths a citation could mean: the exact path if it
    exists, else any real files sharing its basename."""
    if os.path.isfile(os.path.join(REPO, relp)):
        return [relp]
    return list(_BASENAME.get(os.path.basename(relp), []))

def same_file(cit, gt):
    c, g = norm(cit), norm(gt)
    return c == g or c.endswith("/" + g) or g.endswith("/" + c) or os.path.basename(c) == os.path.basename(g)

def load_meta():
    meta = {}
    for qid in os.listdir(BUNDLES):
        mp = os.path.join(BUNDLES, qid, "meta.json")
        if os.path.isfile(mp):
            try:
                meta[qid] = json.load(open(mp))
            except Exception:
                pass
    return meta

def context_tokens(mode, qid, retrieved_tokens, meta):
    m = meta.get(qid, {})
    if mode == "alpha":
        return m.get("alpha_tokens", 0)
    if mode == "beta":
        return m.get("beta_tokens", 0)
    if mode == "delta":
        return m.get("delta_tokens", 0) or m.get("delta_used_tokens", 0)
    return retrieved_tokens or 0  # gamma

def main():
    res_path = os.path.join(ROOT, "results.json")
    data = json.load(open(res_path))
    results = data["results"] if isinstance(data, dict) and "results" in data else data
    meta = load_meta()
    build_basename_index()

    scored = []
    for r in results:
        qid, mode = r["id"], r["mode"]
        gts = r["ground_truth"]
        cits = r.get("citations", []) or []
        # location hit
        hit = False
        for c in cits:
            for g in gts:
                if same_file(c["file"], g["file"]) and overlap(
                    int(c["start_line"]), int(c["end_line"]), int(g["start_line"]), int(g["end_line"])):
                    hit = True
                    break
            if hit:
                break
        # hallucinations (real fabrication) vs self-ref (cited the eval bundle/context file)
        halluc = 0
        bad = []
        selfref = 0
        for c in cits:
            relp = norm(c["file"])
            s, e = int(c["start_line"]), int(c["end_line"])
            # citing the eval scaffolding (its own context bundle) is a citation-quality
            # artifact, not a fabricated code location — count it separately.
            if "eval/ckg-4way/bundles" in relp or relp.endswith(".txt") and "bundles" in relp:
                selfref += 1
                continue
            cands = resolve_candidates(relp)
            if not cands:
                halluc += 1; bad.append(f"{relp}:{s}-{e}(no-file)")
                continue
            # valid if the range fits within ANY real file matching the citation,
            # and is well-formed. Out of range for every candidate => fabricated lines.
            ok = False
            maxn = 0
            for cand in cands:
                n = file_line_count(cand)
                if n is None:
                    continue
                maxn = max(maxn, n)
                if s >= 1 and e >= s and s <= n:
                    ok = True; break
            if not ok:
                halluc += 1; bad.append(f"{relp}:{s}-{e}(range>{maxn})")
        toks = context_tokens(mode, qid, r.get("retrieved_tokens", 0), meta)
        scored.append({
            **r,
            "location_hit": hit,
            "n_citations": len(cits),
            "hallucinations": halluc,
            "selfref_citations": selfref,
            "bad_citations": bad,
            "context_tokens": toks,
        })

    # aggregate by mode
    agg = {}
    by_mode = defaultdict(list)
    for s in scored:
        by_mode[s["mode"]].append(s)
    for mode, rows in by_mode.items():
        n = len(rows)
        agg[mode] = {
            "n": n,
            "location_acc": round(sum(1 for x in rows if x["location_hit"]) / n, 3),
            "correct": sum(1 for x in rows if x["correctness"] == "correct"),
            "partial": sum(1 for x in rows if x["correctness"] == "partial"),
            "incorrect": sum(1 for x in rows if x["correctness"] == "incorrect"),
            "correct_rate": round(sum(1 for x in rows if x["correctness"] == "correct") / n, 3),
            "pass_rate": round(sum(1 for x in rows if x["correctness"] in ("correct", "partial")) / n, 3),
            "total_hallucinations": sum(x["hallucinations"] for x in rows),
            "total_selfref": sum(x["selfref_citations"] for x in rows),
            "total_citations": sum(x["n_citations"] for x in rows),
            "avg_context_tokens": round(sum(x["context_tokens"] for x in rows) / n, 1),
        }
    # correctness by mode x difficulty, and by mode x domain
    def corr_rate(rows):
        return round(sum(1 for x in rows if x["correctness"] == "correct") / len(rows), 3) if rows else None
    by_diff = {}
    by_domain_corr = {}
    for mode, rows in by_mode.items():
        d = {}
        for diff in ("easy", "medium", "hard"):
            rs = [x for x in rows if META.get(x["id"], ("", ""))[1] == diff]
            d[diff] = {"n": len(rs), "correct_rate": corr_rate(rs)}
        by_diff[mode] = d
        dom = {}
        for x in rows:
            dn = META.get(x["id"], ("?", ""))[0]
            dom.setdefault(dn, []).append(x)
        by_domain_corr[mode] = {k: corr_rate(v) for k, v in sorted(dom.items())}

    out = {"by_mode": agg, "by_difficulty": by_diff, "by_domain_correct": by_domain_corr, "scored": scored}
    json.dump(out, open(os.path.join(ROOT, "scored.json"), "w"), indent=2)

    order = ["alpha", "beta", "gamma", "delta"]
    names = {"alpha": "α raw-files", "beta": "β whole-graph", "gamma": "γ on-demand", "delta": "δ auto-select"}
    print(f"{'mode':16} {'n':>3} {'loc-acc':>8} {'correct':>8} {'pass':>6} {'halluc':>7} {'selfref':>7} {'cites':>6} {'avg-tok':>8}")
    for m in order:
        a = agg.get(m)
        if not a:
            continue
        print(f"{names[m]:16} {a['n']:>3} {a['location_acc']:>8} {a['correct_rate']:>8} {a['pass_rate']:>6} "
              f"{a['total_hallucinations']:>7} {a['total_selfref']:>7} {a['total_citations']:>6} {a['avg_context_tokens']:>8}")

    print("\ncorrectness by difficulty (correct-rate):")
    print(f"{'mode':16} {'easy':>14} {'medium':>14} {'hard':>14}")
    for m in order:
        d = by_diff.get(m)
        if not d:
            continue
        def cell(x):
            return f"{x['correct_rate']} (n={x['n']})" if x['correct_rate'] is not None else "-"
        print(f"{names[m]:16} {cell(d['easy']):>14} {cell(d['medium']):>14} {cell(d['hard']):>14}")

if __name__ == "__main__":
    main()
