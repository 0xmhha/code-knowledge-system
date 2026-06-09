#!/usr/bin/env python3
"""Score the change-impact eval: how completely each mode recovers the
ground-truth impact set (the code that must change), split by reachability so
we can see whether the graph finds the *relational* members grep misses.

Inputs:
  gt/CIxx.json            ground-truth impact sets (file, symbol, reach)
  results_impact.json     {results:[{id, mode, impact:[{file,symbol,start_line,end_line}], ...}]}
Outputs:
  scored_impact.json + a console table.
"""
import json, os, glob, re
from collections import defaultdict

ROOT = os.path.dirname(os.path.abspath(__file__))            # .../impact
EVAL = os.path.dirname(ROOT)                                  # .../ckg-4way
REPO = "/Users/wm-it-25_0220/Work/github/go-stablenet"
RELATIONAL = {"transitive", "interface-impl", "semantic"}

def last_comp(sym):
    if not sym:
        return ""
    s = re.split(r"[./]", sym.strip())
    return s[-1].lower() if s else ""

def base(f):
    return os.path.basename((f or "").strip())

def file_match(a, b):
    a, b = (a or "").strip().lstrip("./"), (b or "").strip().lstrip("./")
    if not a or not b:
        return False
    return a == b or a.endswith("/" + b) or b.endswith("/" + a) or base(a) == base(b)

def line_overlap(a, b):
    try:
        return int(a["start_line"]) <= int(b["end_line"]) and int(b["start_line"]) <= int(a["end_line"])
    except Exception:
        return False

def sym_match(a_sym, g_sym):
    la, lg = last_comp(a_sym), last_comp(g_sym)
    if not la or not lg:
        return False
    return la == lg or la in lg or lg in la

def member_match(a, g):
    """Answer member a matches GT member g."""
    if not file_match(a.get("file"), g.get("file")):
        return False
    if g.get("symbol") and sym_match(a.get("symbol"), g.get("symbol")):
        return True
    if all(k in a for k in ("start_line", "end_line")) and all(k in g for k in ("start_line", "end_line")):
        return line_overlap(a, g)
    # same-file, no usable lines/symbols -> count as match (lenient on recall)
    return not g.get("symbol")

# --- repo file index for hallucination check ---
_BN = {}
def build_index():
    skip = {".git", "vendor", "node_modules"}
    for dp, dns, fns in os.walk(REPO):
        dns[:] = [d for d in dns if d not in skip]
        for fn in fns:
            _BN.setdefault(fn, []).append(os.path.relpath(os.path.join(dp, fn), REPO))
_LC = {}
def linecount(rel):
    fp = os.path.join(REPO, rel)
    if fp not in _LC:
        try:
            _LC[fp] = sum(1 for _ in open(fp, "rb"))
        except Exception:
            _LC[fp] = None
    return _LC[fp]
def is_hallucination(a):
    f = (a.get("file") or "").strip().lstrip("./")
    cands = [f] if os.path.isfile(os.path.join(REPO, f)) else _BN.get(base(f), [])
    if not cands:
        return True
    s, e = a.get("start_line"), a.get("end_line")
    if s is None or e is None:
        return False
    for c in cands:
        n = linecount(c)
        if n and int(s) >= 1 and int(e) >= int(s) and int(s) <= n:
            return False
    return True

def main():
    build_index()
    gt = {}
    for f in sorted(glob.glob(os.path.join(ROOT, "gt", "CI*.json"))):
        g = json.load(open(f))
        gt[g["id"]] = g["impact"]
    data = json.load(open(os.path.join(ROOT, "results_impact.json")))
    rows = data["results"] if isinstance(data, dict) else data

    # token map from bundle meta
    toks = {}
    for mf in glob.glob(os.path.join(ROOT, "bundles", "CI*", "meta.json")):
        m = json.load(open(mf))
        toks[m["id"]] = m

    scored = []
    for r in rows:
        members = gt.get(r["id"], [])
        ans = r.get("impact", []) or []
        matched_gt = [g for g in members if any(member_match(a, g) for a in ans)]
        tp_ans = [a for a in ans if any(member_match(a, g) for g in members)]
        rel = [g for g in members if g.get("reach") in RELATIONAL]
        rel_hit = [g for g in rel if g in matched_gt]
        direct = [g for g in members if g.get("reach") not in RELATIONAL]
        dir_hit = [g for g in direct if g in matched_gt]
        halluc = sum(1 for a in ans if is_hallucination(a))
        m = r["mode"]
        tk = toks.get(r["id"], {})
        ctx = (r.get("retrieved_tokens", 0) if m == "gamma"
               else tk.get(f"{m}_tokens", 0))
        scored.append({
            "id": r["id"], "mode": m,
            "n_gt": len(members), "n_ans": len(ans),
            "recall": round(len(matched_gt) / len(members), 3) if members else None,
            "precision": round(len(tp_ans) / len(ans), 3) if ans else 0.0,
            "rel_total": len(rel), "rel_hit": len(rel_hit),
            "dir_total": len(direct), "dir_hit": len(dir_hit),
            "hallucinations": halluc,
            "context_tokens": ctx,
        })

    agg = {}
    by_mode = defaultdict(list)
    for s in scored:
        by_mode[s["mode"]].append(s)
    for mode, rs in by_mode.items():
        def micro(num, den):
            N = sum(x[den] for x in rs); H = sum(x[num] for x in rs)
            return round(H / N, 3) if N else None
        recs = [x["recall"] for x in rs if x["recall"] is not None]
        precs = [x["precision"] for x in rs]
        agg[mode] = {
            "n": len(rs),
            "macro_recall": round(sum(recs) / len(recs), 3) if recs else None,
            "macro_precision": round(sum(precs) / len(precs), 3) if precs else None,
            "micro_recall": micro("dir_hit", "dir_total") if False else None,  # placeholder
            "relational_recall": round(sum(x["rel_hit"] for x in rs) / max(1, sum(x["rel_total"] for x in rs)), 3),
            "direct_recall": round(sum(x["dir_hit"] for x in rs) / max(1, sum(x["dir_total"] for x in rs)), 3),
            "total_hallucinations": sum(x["hallucinations"] for x in rs),
            "avg_ans": round(sum(x["n_ans"] for x in rs) / len(rs), 1),
            "avg_tokens": round(sum(x["context_tokens"] for x in rs) / len(rs), 1),
        }
    json.dump({"by_mode": agg, "scored": scored}, open(os.path.join(ROOT, "scored_impact.json"), "w"), indent=2)

    names = {"alpha": "α grep", "beta": "β subgraph", "gamma": "γ on-demand", "delta": "δ auto-pack"}
    print(f"{'mode':14} {'n':>3} {'recall':>7} {'prec':>6} {'REL-rec':>8} {'DIR-rec':>8} {'halluc':>7} {'avg-ans':>8} {'avg-tok':>8}")
    for m in ["alpha", "beta", "gamma", "delta"]:
        a = agg.get(m)
        if not a:
            continue
        print(f"{names[m]:14} {a['n']:>3} {str(a['macro_recall']):>7} {str(a['macro_precision']):>6} "
              f"{str(a['relational_recall']):>8} {str(a['direct_recall']):>8} {a['total_hallucinations']:>7} "
              f"{a['avg_ans']:>8} {a['avg_tokens']:>8}")
    print("\nREL-rec = recall on relational members (interface-impl/transitive/semantic) that bare grep tends to miss")

if __name__ == "__main__":
    main()
