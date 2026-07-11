#!/usr/bin/env python3
"""
check_corpus.py — corpus.jsonl 구조 정합성 검사 (재현 가능)

검사 항목:
  1) edge 참조 무결성 (calls/calls_flow/enforces/called_by 대상 존재)
  2) flow.links / called_by 대상 존재
  3) step.invariants ID 존재
  4) 불변식 양방향 대칭 (step→INV ↔ INV→step)
  5) enforced_at가 가리키는 step 실존
  6) 고아 INV (강제 지점 없음)
  7) 정밀도 경고: 라인 없는 step (참고용, 실패 아님)

exit code: 문제 0건이면 0, 있으면 1.

usage: python3 tools/check_corpus.py [corpus.jsonl]
"""
import json, sys, os, collections

def main():
    path = sys.argv[1] if len(sys.argv) > 1 else \
        os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "corpus.jsonl")
    recs = [json.loads(l) for l in open(path, encoding="utf-8")]
    flows = {r["id"] for r in recs if r["type"] == "flow"}
    steps = {f"{r['flow']}:{r['id']}" for r in recs if r["type"] == "step"}
    invs  = {r["id"] for r in recs if r["type"] == "invariant"}
    step_recs = [r for r in recs if r["type"] == "step"]
    inv_recs  = [r for r in recs if r["type"] == "invariant"]
    P = []

    for r in recs:
        if r["type"] != "edge": continue
        rel, to = r["rel"], r.get("to")
        if rel == "calls" and to not in steps: P.append(("DANGLING calls", r["from"], to))
        if rel == "calls_flow" and to not in flows and to not in steps: P.append(("DANGLING calls_flow", r["from"], to))
        if rel == "called_by" and to not in flows: P.append(("DANGLING called_by", r["from"], to))
        if rel == "enforces" and to not in steps: P.append(("DANGLING enforces", r["from"], to))

    for r in recs:
        if r["type"] != "flow": continue
        for l in r.get("links", []):
            if l not in flows: P.append(("links 미존재", r["id"], l))
        for c in r.get("called_by", []):
            if c not in flows: P.append(("called_by 미존재", r["id"], c))

    fwd = collections.defaultdict(set); bwd = collections.defaultdict(set)
    for s in step_recs:
        for iv in s["invariants"]:
            if iv not in invs: P.append(("미존재 INV 참조", f"{s['flow']}:{s['id']}", iv))
            fwd[iv].add(f"{s['flow']}:{s['id']}")
    for inv in inv_recs:
        for e in inv["enforced_at"]:
            if e["flow"]:
                bwd[inv["id"]].add(f"{e['flow']}:{e['step']}")
                if f"{e['flow']}:{e['step']}" not in steps:
                    P.append(("enforced_at 미존재 step", inv["id"], f"{e['flow']}:{e['step']}"))
        if not inv["enforced_at"]: P.append(("고아 INV", inv["id"]))
    for iv in invs:
        if fwd[iv] - bwd[iv]: P.append(("비대칭 step→INV", iv, sorted(fwd[iv] - bwd[iv])))
        if bwd[iv] - fwd[iv]: P.append(("비대칭 INV→step", iv, sorted(bwd[iv] - fwd[iv])))

    print(f"구조 정합성 문제: {len(P)}건" + (" ✓ 통과" if not P else ""))
    for p in P: print("  -", *p)

    noline = [f"{r['flow']}:{r['id']}" for r in step_recs if r["file"] and r["line"] is None]
    if noline: print(f"[참고] 라인 없는 step(의도된 집계 가능): {noline}")
    print(f"레코드: {dict(collections.Counter(r['type'] for r in recs))}")
    sys.exit(1 if P else 0)

if __name__ == "__main__":
    main()
