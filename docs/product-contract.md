# Yume product contract

The app is the primary Yume experience; other clients save words into the
same library. The core owns study meaning and behavior; each client adapts
presentation to its platform.

## Shared behavior

- A saved word belongs to the same library across clients. The server owns
  scheduling, review history, card content, tags, and account identity.
- New words are introduced explicitly. Ordinary review sessions fetch
  `new_limit: 0`; a learn action grants more words.
- A rating advances the card only after a successful server write or durable
  offline outbox write. Recoverable failures preserve the rating and token.
- Card actions use the app's words: Improve this card, Explain this word,
  Add an example sentence, Add or fix audio, Correct card details,
  Flag a problem, and Remove card from deck. Under Write it yourself:
  Write my own mnemonic, Add my own example, and Fix the translation or
  explanation.
- Notes render by their server-provided kind. Legacy prefixes are compatibility
  data, never the primary contract for new content.

## Brand and language

The app establishes warm paper, plum ink, coral actions, and the crescent moon.
Other clients keep the palette their surface needs; a video overlay stays dark
with its own single accent. Clients share roles and wording, not colors: one
accent per surface, readable contrast, and word-status colors that carry
learning state, never primary-action emphasis.

Use sentence case and learner language. Explain the next action rather than
requiring knowledge of FSRS, MCP, providers, or deployment.

## First use

- Connected: find a word in Dictionary, save it, then learn it in Review. A
  saved-word confirmation links directly to Review.
- Disconnected: clearly labeled sample cards let the learner explore first.

The empty library provides the introduction; no welcome modal interrupts it.
Tutor help is central to richer cards, while setup belongs at the point of
need rather than blocking the first saved word.

## Next increments

1. Offer a connection link or QR flow so learners do not type server addresses.
2. Show whether a tutor has picked up a request. The help sheet already says
   that requests wait for a tutor and links to the setup guide; the server
   records no tutor activity yet to report more.
3. Move shared study preferences into an account-level contract, with migration
   from mobile settings; keep device controls local.
4. Publish versioned API types and pure card-content helpers from the core,
   then have both clients run the same compatibility fixtures.
5. Gate Home pin updates on both clients' checks against the pinned server.

These increments are planned, not implemented by this document.
