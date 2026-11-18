# Mobile development guide

This is an Expo SDK 56 app that connects to the Go server in `../server`.
Keep it compatible with Expo Go: do not add native modules that require a
custom development build.

## Structure

- `src/theme.ts` holds shared color, spacing, radius, and typography tokens.
- `src/api/types.ts` mirrors server response shapes. Go `omitempty` fields may
  be absent, including arrays.
- `src/api/client.ts` owns HTTP calls and throws `ApiError` for HTTP failures.
- `src/server/ServerContext.tsx` owns connection and account state.
- `src/settings/` syncs the `mobile` settings blob with the active account.
- `src/data/queryStore.ts` caches server queries. Every write must invalidate
  each affected query key.
- `src/screens/useReviewSession.ts` owns review state and scheduling requests.
- `src/screens/review/` contains review presentation and focused hooks.

## Product contracts

- The server owns scheduling. A failed rating must not advance the queue.
- Every rating includes an idempotent `review_id`. Offline replay preserves the
  original review time and detects reviews made by another client.
- Learner-facing due counts use `gatedDueCount`; new cards are introduced only
  through the explicit learn-more action.
- The lemma is the headword. Alternate spellings belong on the card back.
- Suggested words are separate from review cards until added to the deck.
- A card's `sentenceTranslation` and `explanation` come from the card (server
  columns); notes carry a server-set `kind` and are rendered as-is, an
  example's translation in its own field.
- Frequency bands and kanji details come from server endpoints; handle missing
  optional dictionary data gracefully.
- Live data must be labeled separately from sample data.
- The voice credit is `CardMediaInfo.voice` (server `card_media.voice`), shown
  once under the audio row behind the `showVoiceCredit` setting; `🎙` notes
  are accepted only for compatibility.
- Definitions: `definitionLang` (`en` default) orders lookups and swaps the
  card meaning for the server's imported Japanese dictionary (negative entry
  ids).
- Web `Pressable` elements render as buttons, so never nest them.
- Use `ConfirmDialog` instead of `Alert.alert`, which does nothing on web.

## UI conventions

- Use tokens from `src/theme.ts`; do not add inline colors to screens.
- Add icons through `src/components/Icon.tsx`.
- Use sentence case for labels and headings.
- Keep learner-facing errors plain and actionable; do not expose raw HTTP or
  fetch errors.

## Verification

```sh
npx tsc --noEmit
npx jest --runInBand
```
