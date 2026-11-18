# Using Yume

Look up a Japanese word, save it to your library, and review it when it is
due. A connected AI tutor can improve cards, answer requests, and suggest
what to learn next. This guide covers the app's four tabs.

## Your first five minutes

1. Open **Dictionary** and search for a Japanese word.
2. Select **Add to deck**.
3. Open **Review** and select **Learn new words**.
4. Reveal the answer and rate the card.
5. If the card needs something, open its menu and select **Improve this
   card** to leave a request for your tutor, or **Write it yourself**.

## The idea in one minute

- A **word** in your library has a status: Unknown, Learning, Known, or
  Ignored. The frequency coverage numbers count statuses.
- A **card** is a word you chose to study: headword, reading, meaning, a
  sentence, and often audio, a mnemonic, and notes.
- **Reviews** are scheduled by the server with FSRS from your ratings:
  Again, Hard, Good, or Easy.
- **Suggested** words come from your AI tutor or Japanese text you scan. They
  never enter review until you add them.

## Review

The Review tab opens on a moon and a number: the cards due right now. Under
it are the last seven days of reviews and the week ahead.

- **Start review** begins a session with due cards only, never new ones.
- When nothing is due, the button reads **Learn N new words** and releases
  that many never-seen cards. N is the "New words at a time" setting. New
  words come in only when you ask.

During a session:

1. The front shows the headword. Tap **Show answer** (or the card).
2. The back shows the reading, the meaning, the sentence with furigana and
   its translation, example sentences, and a **Listen** button. Tap a
   sentence's speaker to hear it.
3. Rate it. **Again** means you could not recall it; the card comes back
   later in the same session. **Hard** means you got there slowly or only
   partly, and still counts as remembered. **Hard**, **Good**, and **Easy**
   schedule it days or weeks out; the label under each button says how far.
   A Known word survives one miss; a second miss within a month moves it
   back to Learning.
4. A new card shows **New word · just learn it**. Read it, then rate your
   recall honestly.

More on a card:

- **Show explanation** and **Show tutor note** open the same area under
  the meaning. A coral dot marks an unread tutor note.
- **Mnemonic** shows a kanji breakdown. Tap a kanji tile for its sheet:
  meanings, readings, level, the words in your deck that use it, and the
  tutor's story if there is one. The breakdown is open on young cards and
  folded on mature ones; **Collapse mnemonics** in Settings folds it
  everywhere.
- The **⋮** menu holds card actions: flag a problem, remove the card, or
  **Improve this card**. That sheet leaves a request that waits on the card
  until your tutor checks it, or lets you write the mnemonic, an example
  sentence, or a corrected translation and explanation yourself. See
  [Request help from a card](ai-cards.md#request-help-from-a-card).
- **Undo** reverses the last rating. **End** leaves the session; rated cards
  stay rated.
- Keyboard, in a browser: Space or Enter reveals, 1 to 4 rate, Z undoes.

When the queue is empty, the done screen shows the count, any cards marked
Again, and what next: keep going if more came due, learn new words, or stop.

Offline: a session in progress keeps working when the connection drops.
Ratings queue on the device and sync, with their original time, when the
server is back. Opening the app with no connection works over an HTTPS
address, where the app keeps a copy of itself; over plain `http://` it needs
the server. If you have never connected, Review offers a clearly labeled
sample deck to practice on.

## Library

- **Words** shows your words as tiles by frequency band: Top 1,000, 1,000 to
  3,000, and so on down to Rare. Each band's header reads "N of 1,000 known
  · %": how much of that band you have marked Known. Words you have never
  met count as unknown.
- The chips filter: **New** (added, never reviewed), **Learning**, **Known**,
  **Unknown**, **Ignored**, and a band row (**Top 1k**, **to 3k**, ... ,
  **Rare**). Chips combine: Unknown plus Top 1k is "the most common words I
  still don't know". Tap the active chip again to clear it.
- **Search** finds words by lemma, reading, meaning, or sentence. Search and
  edit mode switch to a plain list.
- Tap a word for its popup: definition, examples, and the status buttons.
  Change a status here to tell Yume what you already know.
- The edit button at the end of the search field shows delete buttons on
  each tile.

**Suggested**, the second segment, holds words your tutor pushed or you
mined from a transcript until you decide:

- Swipe right, or tap **Add to deck**, to make a card. It starts as
  Learning.
- Swipe left to **Dismiss**. Marking a word Known or Ignored in its popup
  also retires the suggestion.
- **Scan Japanese text** accepts a transcript, conversation, or article. Tap
  **Find repeated words** to see unfamiliar words that recur, with counts,
  readings, and meanings, then add the ones worth keeping to Suggested. The
  pasted text is processed by your server and is not stored.

## Dictionary

Search JMdict in kana, kanji, or romaji (`neko` finds ねこ; the converted
kana shows as a hint). **EN → 日本語** searches meanings in English instead.

Each entry shows readings, senses, and a frequency badge; **Examples**
expands sentences from the Tanaka Corpus. **Add to deck** saves the entry as
a card and marks the word Learning. **In deck** means you already have it;
tap to remove.

## Settings

- **Cards**: Japanese text size, definitions in English or Japanese (the
  Japanese needs the server's optional Wiktionary dictionary), whether the
  other language also appears in a quieter line underneath (off by default),
  furigana (off, unknown words only, or all kanji), tap to reveal English,
  example sentences on or off.
- **Audio**: auto-play the card when the answer appears; generated audio
  (saved clips, when the card has them) or the device voice; whether the
  VOICEVOX voice is named under the audio.
- **Session**: how many new words the Learn button starts, and whether cards
  marked Again return in the same session.
- **Progress** opens the stats page: streak, reviews per day for twelve
  weeks, and coverage by frequency band.
- **Your data**: **Download my deck** saves your whole deck as an Anki file
  (`.apkg`), with review history, tags and audio. On a self-hosted server,
  **Snapshots** lists the copies of your deck kept on the server and
  **Create snapshot** adds one; ask your tutor to restore one.
- **Account** appears when the server gives each person their own deck: who
  is signed in, and **Sign out**.
- **Connection**: the status of your server and its address. When the app
  is served by the server itself there is nothing to set.
- Tap **Yume** at the bottom for the version and the dictionary credits.

## On your phone

The server serves the app, so there is nothing to download from a store.
Open your server's address on the phone and add it to the home screen; the
[README](../README.md#use-yume-on-your-phone) shows how to give the server
an HTTPS address on your private network.

- iPhone: in Safari, tap Share, then **Add to Home Screen**. Do this before
  you sign in or change settings, then use Yume from the icon. The
  home-screen app keeps its own storage, separate from Safari's, and Safari
  clears the storage of sites you have not used for about a week.
- Android: in Chrome, open the menu and select **Install app**. Chrome
  offers it only on an `https://` address; on `http://` you get **Add to
  Home screen**, which opens Yume in Chrome.

## Your data

Everything lives on your server: the deck, ratings, media, and settings, so
a phone and a laptop share one deck. The app caches settings and the current
due queue on the device so a review keeps going offline (see
[Review](#review)); the server copy always wins.

An AI tutor can add sentences, mnemonics, explanations, audio, and kanji
stories through the server API: see [Cards with an AI tutor](ai-cards.md)
for the workflow and [Card content](card-content.md) for the card format.
