# What a card carries

The contract for card content, shared by the server (stores it), the clients
(render it) and the tutor (writes it through the API). A card's own content
lives in the learner's `cards.db`; mnemonics and kanji stories default to
the server's shared library (see [Shared library](#shared-library)) unless
the card overrides them.

Notes have a `kind`. Nothing is told apart by the first character of a text
field.

| Part | Expected | Who makes it | Where it lives |
|---|---|---|---|
| Lemma, reading, glosses | yes | mining / dictionary save | `cards` |
| Sentence + translation | yes | mined with the card, else the tutor | `cards.sentence`, `cards.translation` |
| Explanation | tricky words only | tutor, on request | `cards.explanation` |
| Audio: word clip + sentence clip | yes | the VOICEVOX batch (`yume-server voicevox`) | `card_media`, each clip naming its `voice` |
| Mnemonic | yes for kanji words | tutor | on the card: `mnemonic_parts` and `mnemonic_story` (one per card) |
| Tutor note | when the learner asks | tutor | `card_notes` kind `tutor` |
| Extra example | nice to have | tutor | `card_notes` kind `example` |
| Triage marker | tutor's own bookkeeping | tutor | `card_notes` kind `triage` (never shown to the learner) |
| Kanji story | nice to have | tutor | shared `kanji_notes` in `content.db`; a deck's own row is an override |

Every table carries `created_at` and `updated_at` (unix seconds) and the
database keeps them: leave them out of writes.

## Card fields

- `translation`: the English of `sentence`. Shown under the sentence; hidden
  behind "Show English" when the learner reveals on tap.
- `explanation`: register, nuance, homophone traps, why the obvious reading
  is wrong. Behind "Show explanation". Write one only when the word is
  tricky; most cards should not have it.
- `mnemonic`: the card's memory aid, structured (see [Mnemonic](#mnemonic)).
  One per card.

`translation` and `explanation` are set with `POST /update_card {card_id,
translation, explanation}` (an empty string clears) or at creation with
`/save_card` and `/import_csv`. `mnemonic` is set only with `/set_mnemonic`.

## Mnemonic

A mnemonic is data, not text. Clients render it and never parse it.

```json
{
  "parts": [{ "kanji": "把", "meaning": "grip" }, { "kanji": "握", "meaning": "clench" }],
  "story": "Grip and clench: 把握 is grasping a situation. 状況を把握する."
}
```

- `parts`: the breakdown, in reading order, zero to six entries. `kanji` is
  one to three CJK characters; a single-kanji word opens into radicals
  (`踊`: `足` foot, `甬` passage). A kana-only word gives an origin part
  (`いっそ`: `一層` one layer more) or none. `meaning` is one to forty
  characters, lower case, no trailing period.
- `known` on a served part (never stored): the part appears in a word the
  deck marks Known, so a client can point out the parts still new to the
  learner. Absent for kana parts.
- `story`: one to three sentences that tie the parts to the word, ending
  with a short usage example. Up to four hundred characters. Plain text:
  no markers, emoji or em dashes.

Write: `POST /set_mnemonic {card_id, parts, story}` validates and replaces
the card's own mnemonic; empty `parts` and `story` clears it. Response
`{ "replaced": true|false }`.

Read: every card payload (`/get_due_cards`, `/list_vocab` rows) carries
`mnemonic` as this object, omitted when the card has none. It is not a note;
`/list_card_notes` never returns one.

Storage: on the card, as `mnemonic_parts` (JSON) and `mnemonic_story`. A
mnemonic without parts is a story-only mnemonic.

Audit: `/mnemonic_audit` classifies cards as `ok`, `no_breakdown` (story-only)
or `missing` over the effective mnemonic (own, else shared; `shared: true` on
those rows).

### Shared library

The server keeps one mnemonic per lemma for every deck it serves, in
`content.db` beside the user decks. A card shows its own mnemonic when it
has one and the shared one otherwise, marked `mnemonic_shared: true`, so the
card's own is an override. Clearing the override (`/set_mnemonic` with empty
`parts` and `story`) is "back to the shared one".

- `POST /shared_mnemonic {lemma}` reads the library; `POST
  /set_shared_mnemonic {lemma, parts, story}` writes it (owner only, same
  validation). A tutor improving a mnemonic writes the shared one unless the
  learner asked for a personal version.
- Kanji stories follow the same pattern: `/set_shared_kanji_note {kanji,
  note}` (owner) writes the shared story, `/set_kanji_note` a deck's own
  override, and `/kanji_info` returns the effective one with `note_shared`,
  plus `components`: the parts the kanji is explained through, taken from
  the mnemonic of its single-kanji word (a part written `= 足 foot` opens
  into 足), each with its own effective story and `known`.
- `yume-server mnemonics share [-user ID]` moves a deck's mnemonics and kanji
  stories into the library where it has none and clears the copies that then
  match it; `mnemonics export FILE` and `mnemonics import FILE [-overwrite]`
  move the library as JSON (`{mnemonics: [{lang, lemma, parts, story}],
  kanji_notes: [{lang, kanji, note}]}`), which is how a starter library
  reaches another install.

## Notes (`card_notes`: `kind`, `note`)

`POST /add_card_note {card_id, kind, note}`. The server refuses a note that
starts with `EN:`: that content is a card field. A note sent without a kind
is classified from the markers older tutors wrote, so old scripts keep
working; new ones should send the kind.

| Kind | What it is |
|---|---|
| `tutor` | A short teaching note the learner sees once; collapsed after the first view |
| `example` | An extra sentence in `note` with its English in `translation` (a note field), rendered under Examples. A tutor always sends the English; a learner's own example, or a sentence saved from a reader, may come without it |
| `triage` | A marker a tutor leaves for itself. Clients hide them |
| `mnemonic` | Not a note; see [Mnemonic](#mnemonic). A note sent with this kind, or text opening with `🎓 Mnemonic`, is refused |

Text is stored as sent, minus the marker older tutors opened a note with
(`🎓`, `📝`, `⚠`); clients render `note` as-is and never read a kind or a
translation out of the text.

## Audio

Two clips per card via `/add_card_media`: label `word` and label `sentence`
(`make_primary: true`). Any engine may generate them; the credit comes from
two fields on the clip:

- `source`: `<engine>:<style or voice id>`, e.g. `voicevox:3`. The part before
  the colon names the engine in credits (`voicevox` renders as VOICEVOX,
  anything else as written).
- `voice`: the credited voice name, e.g. ずんだもん. Required for a credit.

Clients show `<engine> · <voice>` under the audio row (a learner setting can
hide it) and About lists every voice grouped by engine (`/list_media_voices`),
which is what a publisher's attribution notice needs.

For VoiceVox, validate the reading it chose against the card's reading before
saving; open_jtalk misreads kanji. Mark stale clips with the `needs-audio`
system tag.

## Review surface

Any client that reviews cards shows the same card in the same order and uses
the same words, so a learner moving between devices sees one deck, not two.

Card back, top to bottom:

1. audio (word, then sentence)
2. headword
3. reading
4. also-written forms
5. definitions (first language, then the optional second line)
6. sentence with its translation
7. explanation
8. the card's **mnemonic** as a titled block (breakdown tiles, then the story)
9. notes by kind: **tutor** and **example** as text, **triage** never shown

Examples and kanji sheets may follow; layout, density and what starts
collapsed are the client's choice.

Rating buttons: Again · Hard · Good · Easy, each with the interval the rating
would give, measured from when the preview was computed (see `preview_at`).

Card actions, with these exact labels and effects:

| Label | Effect |
|---|---|
| Undo last rating | `/undo_review` (multi-step, today's reviews) |
| Improve this card | learner picks a system tag: `needs-explanation`, `needs-sentence`, `needs-audio`, `needs-fix`, or writes the content under **Write it yourself** (below) |
| Show / Hide furigana on the front | toggles the `furigana` tag |
| Flag a problem / Unflag this card | toggles the `flagged` tag |
| Remove card from deck | `/delete_card` |

**Write it yourself**, in the same sheet, lets a learner without a tutor
fill the card through the endpoints a tutor uses:

| Label | Effect |
|---|---|
| Write my own mnemonic | `/set_mnemonic` with the card's current parts and the learner's story (a story-only mnemonic when there are no parts); replaces the shared one on this card only |
| Add my own example | `/add_card_note` kind `example`, the English in `translation` |
| Fix the translation or explanation | `/update_card {translation, explanation}`; the translation only when the card has a sentence; empty clears |

A review surface reads notes and does not delete them.

Cloze cards (`card_type: cloze`) hide `cloze_target` inside the sentence on
the front and reveal it on the back. The server picks the target (the saved
surface form, else the lemma, else a kanji spelling, whichever the sentence
contains) so both surfaces blank the same text; a client that cannot render
cloze must still show the sentence rather than a blank card.

## System tags (`/tag_card`)

Internal triage; never shown as content.

- `flagged`: the learner says something is wrong.
- `needs-explanation`: the learner wants more teaching. Set `explanation`,
  then clear the tag.
- `needs-sentence`, `needs-audio`, `needs-fix`.

`furigana` is the exception the learner sees: the card front shows the
reading over each kanji (from `reading_parts`), the word is shown as "known
by sound" and stays Learning. A card gets it on its 8th lapse and again on
any later miss; `try_kanji` marks one that has held for 30 days, so the
back offers to drop it. Turning it on demotes a Known word to Learning.

## Writing

`POST /update_card {card_id, sentence?, translation?, explanation?}` ·
`/set_mnemonic {card_id, parts, story}` · `/add_card_note {card_id, kind,
note}` · `/delete_card_note {note_id}` · `/set_kanji_note {kanji, note}` ·
`/add_card_media` · `/tag_card`. As the owner, send `X-Yume-User: owner` (open
mode) or the bearer token. Find gaps with `/list_vocab` filters
(`needs_audio`, `new`), `/mnemonic_audit {kind: "missing"}` and
`{kind: "no_breakdown"}`, and rows whose `translation` is empty.

Send request bodies as UTF-8, from a file or a script rather than inline
shell arguments. The server refuses a Japanese field (lemma, kanji, readings,
sentence, note, mnemonic story, kanji story) that is only `?` or contains
U+FFFD, the two ways a shell loses Japanese on Windows.
