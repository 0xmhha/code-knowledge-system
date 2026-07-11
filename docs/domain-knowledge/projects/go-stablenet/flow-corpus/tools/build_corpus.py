#!/usr/bin/env python3
"""
build_corpus.py — flows/*.md + invariants.md → corpus.jsonl

사람이 쓴 코퍼스 markdown을 기계 적재용 JSONL로 변환한다(재현 가능).
스키마는 ../SCHEMA.md 참조. best-effort 파서이며, 형식을 벗어난 부분은 stderr로 경고한다.

usage:  python3 tools/build_corpus.py            # corpus/ 기준 실행
        python3 tools/build_corpus.py --out x.jsonl
"""
import argparse, json, os, re, sys

HERE = os.path.dirname(os.path.abspath(__file__))
CORPUS = os.path.dirname(HERE)               # .../corpus
FLOWS = os.path.join(CORPUS, "flows")

def warn(msg): print(f"[warn] {msg}", file=sys.stderr)

def strip_comment(s):
    # 인라인 ' # ...' 주석 제거 (URL/해시 충돌 줄이려 ' #' 기준)
    i = s.find(" #")
    return (s[:i] if i >= 0 else s).strip()

def parse_list(s):
    s = strip_comment(s).strip()
    if s in ("", "—", "[]"): return []
    m = re.match(r"^\[(.*)\]$", s)
    if m:
        body = m.group(1).strip()
        if not body: return []
        return [x.strip() for x in body.split(",") if x.strip()]
    return [s]

def backtick(s):
    m = re.search(r"`([^`]+)`", s)
    return m.group(1).strip() if m else strip_comment(s).strip()

def split_symbol_loc(at):
    # "core/txpool/validation.go:236" -> ("core/txpool/validation.go", 236)
    at = backtick(at)
    m = re.match(r"^(.*?):(\d+)\s*$", at)
    if m: return m.group(1), int(m.group(2))
    return at, None

# ---------- front matter ----------
def parse_front_matter(text):
    fm = {}
    m = re.match(r"^---\n(.*?)\n---\n", text, re.S)
    if not m: return fm, text
    body = m.group(1)
    for line in body.splitlines():
        if ":" not in line: continue
        k, v = line.split(":", 1)
        k = k.strip(); v = v.strip()
        if k in ("links", "called_by"):
            fm[k] = parse_list(v)
        else:
            fm[k] = strip_comment(v).strip().strip('"')
    return fm, text[m.end():]

# ---------- step blocks ----------
STEP_HDR = re.compile(r"^###\s+STEP\s+([A-Za-z0-9\-]+)", re.M)

def parse_steps(body, flow_id):
    steps = []
    idxs = [(m.start(), m.group(1)) for m in STEP_HDR.finditer(body)]
    for i, (start, sid) in enumerate(idxs):
        end = idxs[i+1][0] if i+1 < len(idxs) else len(body)
        block = body[start:end]
        step = {"type":"step","id":sid,"flow":flow_id,
                "symbol":None,"file":None,"line":None,"kind":None,
                "calls":[],"reads":None,"writes":None,"emits":None,
                "branches":[],"invariants":[],"prose":None}
        lines = block.splitlines()
        n = 0
        while n < len(lines):
            ln = lines[n]
            fm = re.match(r"^- (\w+):\s*(.*)$", ln)
            if not fm:
                n += 1; continue
            key, val = fm.group(1), fm.group(2)
            if key == "symbol":
                step["symbol"] = backtick(val)
            elif key == "at":
                step["file"], step["line"] = split_symbol_loc(val)
            elif key == "kind":
                step["kind"] = strip_comment(val).strip()
            elif key == "calls":
                step["calls"] = parse_list(val)
            elif key in ("reads","writes","emits","prose"):
                step[key] = strip_comment(val).strip() if key != "prose" else val.strip()
            elif key == "invariant":
                step["invariants"] = [x for x in parse_list(val) if x.startswith("INV-")]
            elif key == "branches":
                # 다음 줄들의 '  - when: ... → then: ... at: ...' 수집
                n += 1
                while n < len(lines):
                    b = lines[n]
                    if re.match(r"^- \w+:", b) or b.startswith("###"):
                        break
                    bm = re.match(r"^\s*-\s*when:\s*(.*)$", b)
                    if bm:
                        seg = bm.group(1)
                        when = then = at = None
                        # when: "..." → then: "..." at: `...`
                        wm = re.match(r'^"?(.*?)"?\s*→\s*then:\s*"?(.*?)"?\s*at:\s*(.*)$', seg)
                        if wm:
                            when = wm.group(1).strip().strip('"')
                            then = wm.group(2).strip().strip('"')
                            at = backtick(wm.group(3))
                        else:
                            # arrow 없을 수도
                            when = seg.strip().strip('"')
                        step["branches"].append({"when":when,"then":then,"at":at})
                    n += 1
                continue
            n += 1
        if not step["symbol"]:
            warn(f"{flow_id}:{sid} symbol 누락")
        steps.append(step)
    return steps

# ---------- invariants.md ----------
INV_HDR = re.compile(r"^###\s+(INV-[A-Z]+-\d+)\s*—\s*(.*)$", re.M)

def parse_invariants(text):
    invs = []
    idxs = [(m.start(), m.group(1), m.group(2)) for m in INV_HDR.finditer(text)]
    for i, (start, iid, title) in enumerate(idxs):
        end = idxs[i+1][0] if i+1 < len(idxs) else len(text)
        block = text[start:end]
        domain = iid.split("-")[1]
        inv = {"type":"invariant","id":iid,"domain":domain,"title":title.strip(),
               "statement":None,"assumes":None,"enforced_at":[],"check":None}
        lines = block.splitlines()
        n = 0
        while n < len(lines):
            ln = lines[n]
            fm = re.match(r"^- (\w+):\s*(.*)$", ln)
            if fm:
                key, val = fm.group(1), fm.group(2)
                if key in ("statement","assumes","check"):
                    inv[key] = val.strip()
                elif key == "enforced_at":
                    n += 1
                    while n < len(lines):
                        e = lines[n]
                        if re.match(r"^- \w+:", e) or e.startswith("###"):
                            break
                        if re.match(r"^\s*-\s*`", e):
                            # 한 bullet에 flow:step ref가 여러 개일 수 있다 — 모두 포착
                            allbt = re.findall(r"`([^`]+)`", e)
                            refs = [b.strip() for b in allbt
                                    if re.match(r"^[A-Za-z0-9\-]+:[A-Za-z0-9\-]+$", b.strip())]
                            locs = [b.strip() for b in allbt if b.strip() not in refs]
                            loc = locs[0] if locs else None
                            for ref in refs:
                                fl, st = ref.split(":", 1)
                                inv["enforced_at"].append({"flow":fl,"step":st,"loc":loc})
                        n += 1
                    continue
            n += 1
        invs.append(inv)
    return invs

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", default=os.path.join(CORPUS, "corpus.jsonl"))
    args = ap.parse_args()

    records = []
    n_flow = n_step = n_inv = n_edge = 0

    for fn in sorted(os.listdir(FLOWS)):
        if not fn.endswith(".md"): continue
        text = open(os.path.join(FLOWS, fn), encoding="utf-8").read()
        fm, body = parse_front_matter(text)
        flow_id = fm.get("flow") or fn[:-3]
        flow_rec = {"type":"flow","id":flow_id,
                    "entry_point":fm.get("entry_point"),"trigger":fm.get("trigger"),
                    "summary":fm.get("summary"),"root_symbol":fm.get("root_symbol"),
                    "links":fm.get("links",[]),"called_by":fm.get("called_by",[])}
        records.append(flow_rec); n_flow += 1
        for tgt in flow_rec["called_by"]:
            records.append({"type":"edge","rel":"called_by","from":flow_id,"to":tgt}); n_edge += 1

        for step in parse_steps(body, flow_id):
            records.append(step); n_step += 1
            node = f"{flow_id}:{step['id']}"
            for c in step["calls"]:
                if "-" in c and not c.startswith("ep-") and not c.startswith("spine-"):
                    # 같은 flow의 step id로 간주
                    records.append({"type":"edge","rel":"calls","from":node,"to":f"{flow_id}:{c}"})
                else:
                    records.append({"type":"edge","rel":"calls_flow","from":node,"to":c})
                n_edge += 1

    inv_text = open(os.path.join(CORPUS,"invariants.md"),encoding="utf-8").read()
    for inv in parse_invariants(inv_text):
        records.append(inv); n_inv += 1
        for e in inv["enforced_at"]:
            if e["flow"]:
                records.append({"type":"edge","rel":"enforces","from":inv["id"],
                                "to":f"{e['flow']}:{e['step']}"}); n_edge += 1

    with open(args.out,"w",encoding="utf-8") as f:
        for r in records:
            f.write(json.dumps(r, ensure_ascii=False) + "\n")

    print(f"wrote {args.out}")
    print(f"  flow={n_flow} step={n_step} invariant={n_inv} edge={n_edge} total={len(records)}")

if __name__ == "__main__":
    main()
