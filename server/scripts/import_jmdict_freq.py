#!/usr/bin/env python3
"""Build a frequency dictionary from JMdict's nfXX priority bands and import it
into a running yume-server via the dict-import endpoints.

JMdict tags common headwords/readings with priority codes; the nfXX bands
(nf01..nf48, ~500 words each, from the Mainichi Shimbun corpus) are a ready-made
frequency ranking. We map each entry's best (lowest) band to value = band*500
(lower = more common, matching the server's MIN(value) "rank" semantics) and key
the rows by every kanji + reading surface form so forward lookups annotate freq.

Frequency data is per-user (lives in that user's cards.db), so this imports into
one account. Idempotency: re-running ADDS another dict; delete the old one first
via /list_dicts + /delete_dict if you re-import.

Usage:
  YUME_HOST=http://your-pi:7654 YUME_USER=owner \
  YUME_JMDICT=$HOME/yume/data/JMdict_e.gz \
  python3 import_jmdict_freq.py
"""
import gzip
import json
import os
import re
import sys
import urllib.request
import xml.etree.ElementTree as ET

HOST = os.environ.get("YUME_HOST", "http://127.0.0.1:7654")
USER = os.environ.get("YUME_USER", "owner")
JMDICT = os.environ.get("YUME_JMDICT", "data/JMdict_e.gz")
NF = re.compile(r"nf(\d{2})")
CHUNK = 5000


def post(path, payload):
    req = urllib.request.Request(
        HOST + path,
        data=json.dumps(payload).encode(),
        headers={"Content-Type": "application/json", "X-Yume-User": USER},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=60) as r:
        return json.load(r)


def build_rows(path):
    rows, seen = [], set()
    with gzip.open(path, "rb") as f:
        for _, el in ET.iterparse(f, events=("end",)):
            if el.tag != "entry":
                continue
            kebs = [k.findtext("keb") for k in el.findall("k_ele")]
            rebs = [r.findtext("reb") for r in el.findall("r_ele")]
            pris = [p.text for p in el.findall(".//ke_pri")] + [
                p.text for p in el.findall(".//re_pri")
            ]
            bands = [int(m.group(1)) for p in pris if p for m in [NF.match(p)] if m]
            el.clear()
            if not bands:
                continue
            value = min(bands) * 500
            primary = rebs[0] if rebs else ""
            for form in [x for x in (kebs or []) if x] + [x for x in rebs if x]:
                if form in seen:
                    continue
                seen.add(form)
                rows.append(
                    {
                        "expression": form,
                        "reading": primary if form in (kebs or []) else form,
                        "value": value,
                        "display": f"nf{min(bands):02d}",
                    }
                )
    return rows


def main():
    rows = build_rows(JMDICT)
    print(f"built {len(rows)} freq rows from {JMDICT}", flush=True)
    did = post(
        "/create_dict",
        {
            "name": "JMdict frequency (nf bands)",
            "author": "JMdict/EDRDG",
            "description": "Frequency rank from JMdict nfXX priority bands. Lower = more common.",
            "url": "https://www.edrdg.org/jmdict/j_jmdict.html",
        },
    )["dict_id"]
    print("dict_id =", did, flush=True)
    total = 0
    for i in range(0, len(rows), CHUNK):
        total += post("/import_freq_chunk", {"dict_id": did, "freqs": rows[i : i + CHUNK]})[
            "imported"
        ]
        print(f"  imported {total}/{len(rows)}", flush=True)
    print("DONE:", total, "rows imported into user", USER)


if __name__ == "__main__":
    sys.exit(main())
