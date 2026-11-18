# First-word journey

`npm run test:journey` exports the app and runs a headless browser against a
local fixture API. It never writes to a real Yume deck.

Install a browser once with `npx playwright install chromium`, or set
`YUME_BROWSER_CHANNEL=msedge` to use installed Edge.

The fixture covers empty library, dictionary search, saving, the Review
handoff, confirmed/cancelled removal, the first rating, and Settings
controls. It also checks that a returning learner does not see the
empty-library introduction. Screenshots go to the ignored `e2e/artifacts/`
directory.
