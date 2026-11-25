"""Turn ProCyclingStats pages the user saved in a browser into one CSV.

Reads every saved page (Safari .webarchive or .html) in a folder and writes
rows Race,Kind,Pos,Rider,Team,GapS,Status:
  - a one-day result page: every finisher with his gap, plus DNF/DNS/OTL;
  - a stage page: the general classification after that stage (save the last
    stage for the final GC);
  - a start list: every starter (Kind "start", no position).
Riders are written "Given Surname" (PCS prints "SURNAME Given").

Usage: python3 -I tools/pcs_saved_pages.py data/local/results_2026 data/local/results_2026.csv

The pages are PCS's: keep them and the CSV in data/local/ (gitignored).
"""

import csv
import glob
import html
import os
import plistlib
import re
import sys


def page_html(path):
    if path.endswith(".webarchive"):
        with open(path, "rb") as f:
            d = plistlib.load(f)
        return d["WebMainResource"]["WebResourceData"].decode("utf-8", "replace")
    with open(path, encoding="utf-8", errors="replace") as f:
        return f.read()


def text(fragment):
    return html.unescape(re.sub(r"\s+", " ", re.sub(r"<[^>]+>", " ", fragment))).strip()


def given_first(pcs_name):
    """'VAN DER POEL Mathieu' -> 'Mathieu Van Der Poel'."""
    words = pcs_name.split()
    surname = [w for w in words if w.upper() == w and any(c.isalpha() for c in w)]
    given = [w for w in words if w not in surname]
    return " ".join(given + [w.capitalize() if "-" not in w else "-".join(p.capitalize() for p in w.split("-")) for w in surname]).strip()


def clock(s):
    """'6:20:07' or '0:34' or '+ 1:02' -> seconds; '' -> None."""
    s = s.replace("+", "").strip()
    m = re.fullmatch(r"(\d+):(\d\d)(?::(\d\d))?", s)
    if not m:
        return None
    a, b, c = m.groups()
    return int(a) * 3600 + int(b) * 60 + int(c) if c else int(a) * 60 + int(b)


def tables(page):
    for m in re.finditer(r"<table", page):
        end = page.find("</table>", m.start())
        t = page[m.start():end]
        heads = [text(h) for h in re.findall(r"<th[^>]*>(.*?)</th>", t, re.S)]
        rows = []
        for r in re.findall(r"<tr[^>]*>(.*?)</tr>", t, re.S):
            cells = re.findall(r"<td[^>]*>(.*?)</td>", r, re.S)
            if cells:
                rows.append(cells)
        yield heads, rows


def result_rows(rows):
    """Rank, rider, team, gap (s) and status from a result table's rows.

    PCS shows the winner's total time and everyone else's gap in the column
    before the trailing '..'; an empty or ',,' gap means the same time as the
    rider above."""
    out, last_gap = [], 0
    for i, cells in enumerate(rows):
        rank = text(cells[0])
        rider_cell = next((c for c in cells if 'href="rider/' in c), None)
        if rider_cell is None:
            continue
        name = text(re.search(r'<a[^>]*href="rider/[^"]*"[^>]*>(.*?)</a>', rider_cell, re.S).group(1))
        team_m = re.search(r'href="team/[^"]*"[^>]*>(.*?)</a>', "".join(cells), re.S)
        team = text(team_m.group(1)) if team_m else ""
        time_cell = text(cells[-2]) if len(cells) >= 2 else ""
        time_cell = time_cell.split()[0] if time_cell else ""
        status = ""
        if not rank.isdigit():
            status, rank, gap = rank, "", ""
        else:
            secs = clock(time_cell)
            if i == 0 or int(rank) == 1:
                gap, last_gap = 0, 0
            elif secs is None:
                gap = last_gap
            else:
                gap, last_gap = secs, secs
        out.append((rank, given_first(name), team, gap, status))
    return out


def parse(path):
    page = page_html(path)
    title = text(re.search(r"<title>(.*?)</title>", page, re.S).group(1))
    if title.startswith("Startlist for"):
        race = title[len("Startlist for "):]
        block = page[page.find("startlist_v4"):]
        rows, team = [], ""
        for m in re.finditer(r'href="(team|rider)/[^"]*"[^>]*>(.*?)</a>', block, re.S):
            kind, label = m.group(1), text(m.group(2))
            if kind == "team":
                team = re.sub(r"\s*\((WT|PRT|CT)\)\s*$", "", label)
            elif label and any(w.upper() == w and any(c.isalpha() for c in w) for w in label.split()):
                # Start-list entries print the surname in capitals; other
                # rider links on the page (sidebars) don't.
                rows.append((race, "start", "", given_first(label), team, "", ""))
        return rows
    race = re.sub(r"\s+(One day race|Stage \d+) results$", "", title)
    stage = re.search(r"Stage (\d+) results$", title)
    found = list(tables(page))
    if stage:
        # The general classification after this stage: the table with the
        # previous-place column (the stage result has none).
        gc = next((rows for heads, rows in found if "Prev." in heads and "UCI" in heads), None)
        if gc is None:
            return []
        return [(race, "gc", *r) for r in result_rows(gc)]
    one = next((rows for heads, rows in found if "Rider" in heads), None)
    return [(race, "result", *r) for r in result_rows(one or [])]


def main(folder, out):
    rows = []
    for path in sorted(glob.glob(os.path.join(folder, "*.webarchive")) + glob.glob(os.path.join(folder, "*.htm*"))):
        got = parse(path)
        print(f"{os.path.basename(path)[:70]:70} {len(got):4} rows")
        rows.extend(got)
    with open(out, "w", newline="", encoding="utf-8") as f:
        w = csv.writer(f)
        w.writerow(["Race", "Kind", "Pos", "Rider", "Team", "GapS", "Status"])
        w.writerows(rows)
    print(f"wrote {len(rows)} rows to {out}")


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
