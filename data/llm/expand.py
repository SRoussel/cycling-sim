#!/usr/bin/env python3
"""Expand data/llm/draft_spec.txt into data/rider_llm.csv (a rider-overrides layer).

The spec holds LLM-drafted physical profiles: a specialty type, weight and
threshold W/kg per rider, plus optional per-rider column overrides. Run from
the repo root:  python3 data/llm/expand.py
"""
import csv
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
DATA = os.path.dirname(HERE)

# Type defaults: Sprint_watts, Climbing, Sprint, TT (+ Handling for cobbled classics).
TYPES = {
    "GC": {"Sprint_watts": 1200, "Climbing": 84, "Sprint": 58, "TT": 78},
    "CL": {"Sprint_watts": 1200, "Climbing": 80, "Sprint": 60, "TT": 72},
    "PU": {"Sprint_watts": 1550, "Climbing": 76, "Sprint": 76, "TT": 74},
    "SP": {"Sprint_watts": 1800, "Climbing": 58, "Sprint": 84, "TT": 64},
    "LO": {"Sprint_watts": 1700, "Climbing": 60, "Sprint": 80, "TT": 70},
    "CX": {"Sprint_watts": 1600, "Climbing": 66, "Sprint": 76, "TT": 76, "Handling": 84},
    "TT": {"Sprint_watts": 1450, "Climbing": 66, "Sprint": 64, "TT": 86},
    "RO": {"Sprint_watts": 1450, "Climbing": 66, "Sprint": 68, "TT": 74},
}
NAMES = {"GC": "grand-tour climber", "CL": "climber", "PU": "puncheur", "SP": "sprinter",
         "LO": "lead-out", "CX": "classics", "TT": "time triallist", "RO": "rouleur"}
COLUMNS = ["Weight_kg", "FTP_watts", "Sprint_watts", "Climbing", "Sprint", "TT", "Handling"]
CONFIDENCE = {"high", "medium", "low"}


def main():
    names, base = {}, {}
    with open(os.path.join(DATA, "riders.csv"), newline="") as f:
        for i, row in enumerate(csv.DictReader(f), start=1):
            names[i] = row["Name"]  # IDs follow row order (as seeded)
            base[i] = (float(row["Weight_kg"]), float(row["FTP_watts"]))

    out_rows, seen, movers = [], set(), []
    with open(os.path.join(HERE, "draft_spec.txt")) as f:
        for n, line in enumerate(f, start=1):
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            parts = line.split("|")
            if len(parts) not in (5, 6):
                sys.exit(f"draft_spec.txt:{n}: want id|type|kg|wkg|confidence[|overrides]")
            rid, typ, kg, wkg, conf = int(parts[0]), parts[1], float(parts[2]), float(parts[3]), parts[4]
            if typ not in TYPES or conf not in CONFIDENCE or rid not in names or rid in seen:
                sys.exit(f"draft_spec.txt:{n}: bad type/confidence/id: {line}")
            if not (45 <= kg <= 95 and 4.5 <= wkg <= 7.0):
                sys.exit(f"draft_spec.txt:{n}: implausible weight or W/kg: {line}")
            seen.add(rid)
            bkg, bftp = base[rid]
            movers.append((abs(wkg - bftp / bkg), names[rid], bftp / bkg, wkg, NAMES[typ], conf))
            vals = dict(TYPES[typ])
            vals["Weight_kg"] = kg
            vals["FTP_watts"] = round(kg * wkg)
            if len(parts) == 6 and parts[5]:
                for kv in parts[5].split(","):
                    k, v = kv.split("=")
                    if k not in COLUMNS:
                        sys.exit(f"draft_spec.txt:{n}: unknown column {k}")
                    vals[k] = float(v) if k == "Weight_kg" else int(v)
            row = {"Name": names[rid], "Confidence": conf,
                   "Source": f"llm-draft: {NAMES[typ]}, {wkg:.2f} W/kg"}
            for c in COLUMNS:
                v = vals.get(c, "")
                row[c] = (f"{v:g}" if isinstance(v, float) else str(v)) if v != "" else ""
            out_rows.append(row)

    out = os.path.join(DATA, "rider_llm.csv")
    with open(out, "w", newline="") as f:
        f.write("# LLM-drafted physical profiles (weight, threshold W/kg, specialty), generated\n")
        f.write("# offline by data/llm/expand.py from data/llm/draft_spec.txt. Drafted from general\n")
        f.write("# knowledge of each rider's build and specialty; no race results were used as\n")
        f.write("# inputs, but an LLM's sense of a specialty is itself shaped by results it has seen.\n")
        f.write("# Applied right after riders.csv; rider_derived.csv, rider_overrides.csv and\n")
        f.write("# rider_fitted.csv all override it. Review high-impact rows before trusting them.\n")
        w = csv.DictWriter(f, fieldnames=["Name"] + COLUMNS + ["Confidence", "Source"])
        w.writeheader()
        w.writerows(out_rows)
    counts = {c: sum(r["Confidence"] == c for r in out_rows) for c in ("high", "medium", "low")}
    print(f"wrote {out}: {len(out_rows)} riders ({counts})")
    if "--review" in sys.argv:
        print("\nBiggest W/kg changes vs riders.csv (review these first):")
        for d, name, old, new, kind, conf in sorted(movers, reverse=True)[:25]:
            print(f"  {name:30s} {old:4.2f} -> {new:4.2f} W/kg  {kind:18s} ({conf})")


if __name__ == "__main__":
    main()
