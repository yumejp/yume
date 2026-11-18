#!/usr/bin/env python3
"""Generate VoiceVox audio for every card that has none.

Per card: a word clip (reading-checked) and a sentence clip (made primary),
each carrying the voice name as its credit. Safe to re-run: it asks the
server for cards that still lack audio.

Environment:
  YUME_URL    server, default http://127.0.0.1:7654
  YUME_USER   X-Yume-User header in open auth mode, default owner
  YUME_TOKEN  bearer token instead (accounts mode)
  VOICE_STYLES VoiceVox style ids, comma-separated, default 3 (ずんだもん
              ノーマル). With several, a card's id picks its voice, so a
              re-run gives the same card the same voice.
The credit stored on each clip is the speaker's name from the model file, so
it cannot disagree with the audio.
Run with the venv python from the same folder as core/ (yume-server voicevox
sets both up).
"""
import base64
import glob
import json
import os
import re
import sys
import time
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
BASE = os.environ.get("YUME_URL", "http://127.0.0.1:7654").rstrip("/")
STYLES = [int(x) for x in os.environ.get("VOICE_STYLES", os.environ.get("VOICE_STYLE", "3")).split(",") if x.strip()]
HEADERS = {"Content-Type": "application/json"}
if os.environ.get("YUME_TOKEN"):
    HEADERS["Authorization"] = "Bearer " + os.environ["YUME_TOKEN"]
else:
    HEADERS["X-Yume-User"] = os.environ.get("YUME_USER", "owner")


def post(path, body):
    req = urllib.request.Request(BASE + path, json.dumps(body).encode(), HEADERS)
    with urllib.request.urlopen(req, timeout=60) as r:
        return json.load(r)


KATA = str.maketrans({chr(k): chr(k - 0x60) for k in range(0x30A1, 0x30F7)})
O_ROW = "おこそとのほもよろをごぞどぼぽょ"
E_ROW = "えけせてねへめれげぜでべぺ"


def normalize_kana(s):
    # VoiceVox reports AquesTalk-style katakana with phonetic long vowels
    # (ぎょおじ for ぎょうじ); fold both sides so readings compare equal.
    s = re.sub(r"[^ぁ-ゖー]", "", s.translate(KATA))
    s = re.sub(f"(?<=[{O_ROW}])[うお]", "お", s)
    s = re.sub(f"(?<=[{E_ROW}])[いえ]", "え", s)
    return s.replace("ー", "")


def cards_without_audio():
    out, offset = [], 0
    while True:
        page = post("/list_vocab", {"filter": {"needs_audio": True}, "limit": 500, "offset": offset})
        rows = page.get("rows") or []
        out += [(r["card_id"], r["lemma"], r.get("readings") or [], r.get("sentence") or "") for r in rows if r.get("card_id")]
        offset += len(rows)
        if not rows or offset >= page.get("total", 0):
            return out


def find(pattern):
    hits = sorted(glob.glob(os.path.join(HERE, "core", pattern), recursive=True))
    if not hits:
        sys.exit(f"missing {pattern} under {HERE}/core; run `yume-server voicevox`")
    return hits[-1]


cards = cards_without_audio()
print(f"{len(cards)} cards to voice", flush=True)
if not cards:
    sys.exit(0)

from voicevox_core.blocking import Onnxruntime, OpenJtalk, Synthesizer, VoiceModelFile  # noqa: E402

ort = Onnxruntime.load_once(filename=find("onnxruntime/lib/libvoicevox_onnxruntime*") if os.name != "nt" else find("onnxruntime/lib/voicevox_onnxruntime*.dll"))
synth = Synthesizer(ort, OpenJtalk(find("dict/open_jtalk_dic*")))
# Load only the model files that hold a wanted style; name each style after
# its speaker ("ずんだもん", or "ずんだもん(あまあま)" for a non-default style).
NAMES = {}
for path in sorted(glob.glob(os.path.join(HERE, "core", "models", "vvms", "*.vvm"))):
    with VoiceModelFile.open(path) as m:
        held = {s.id: (meta.name if s.name == "ノーマル" else f"{meta.name}({s.name})") for meta in m.metas for s in meta.styles}
        if any(st in held for st in STYLES):
            synth.load_voice_model(m)
            NAMES.update({st: held[st] for st in STYLES if st in held})
missing = [st for st in STYLES if st not in NAMES]
if missing:
    sys.exit(f"no model for style {missing}; see `yume-server voicevox`")
print("voices: " + ", ".join(f"{st} {NAMES[st]}" for st in STYLES), flush=True)


def attach(cid, label, wav, primary, style):
    post("/add_card_media", {
        "card_id": cid, "kind": "audio", "label": label, "source": f"voicevox:{style}",
        "voice": NAMES[style],
        "media": {"mime": "audio/wav", "data": base64.b64encode(wav).decode()},
        "make_primary": primary,
    })


done = failed = 0
for cid, lemma, readings_raw, sentence in cards:
    try:
        if not sentence:
            print(f"{cid} {lemma}: skip, no sentence", flush=True)
            continue
        STYLE = STYLES[cid % len(STYLES)]
        readings = [normalize_kana(r) for r in readings_raw]
        word_text, note = lemma, ""
        heard = normalize_kana(synth.create_audio_query(lemma, STYLE).kana)
        if readings and heard not in readings:
            word_text, note = readings_raw[0], f" (misread {heard!r}, used kana)"
        t0 = time.time()
        word_wav = synth.tts(word_text, STYLE)
        sent_text = sentence
        if readings and lemma in sentence:
            heard_sent = normalize_kana(synth.create_audio_query(sentence, STYLE).kana)
            stems = [r for r in readings if len(r) >= 2] + [r[:-1] for r in readings if len(r) >= 3]
            if not any(s in heard_sent for s in stems):
                sent_text = sentence.replace(lemma, readings_raw[0])
                note += " (sentence misread, kana substituted)"
        sent_wav = synth.tts(sent_text, STYLE)
        attach(cid, "word", word_wav, False, STYLE)
        attach(cid, "sentence", sent_wav, True, STYLE)
        done += 1
        print(f"{cid} {lemma}: ok {NAMES[STYLE]} {time.time() - t0:.0f}s{note}", flush=True)
    except Exception as e:  # noqa: BLE001
        failed += 1
        print(f"{cid} {lemma}: FAIL {e}", flush=True)
print(f"done={done} failed={failed}", flush=True)
sys.exit(1 if failed else 0)
