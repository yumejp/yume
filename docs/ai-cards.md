# Cards with an AI tutor

Yume stores and schedules your cards. A connected AI tutor can inspect the
deck, respond to requests you leave on cards, fill missing content, and suggest
useful words. It does not rate cards or change your review schedule.

## How the connection works

Yume exposes two local interfaces:

- **MCP** lets the tutor read learning summaries, flagged cards, suggestions,
  tags, and backups, and suggest words and flag cards. Video tools are filled
  only by the Yume browser extension.
- **HTTP API** lets the tutor add or replace sentences, mnemonics, explanations,
  kanji notes, tags, and media.

The bundled `yume-sensei` skill describes the workflow and the content format
the app expects. Your AI client supplies the model; Yume does not send the
deck to a separate hosted AI service.

What a client can do depends on what it can reach:

- **Full tutor.** An agent that can make HTTP requests or run commands on the
  machine running Yume or on the same private network: Claude Code, the Codex
  CLI, the Codex IDE extension, or Codex in the ChatGPT desktop app.
- **Read and suggest only.** A chat app that only speaks MCP, such as Claude
  Desktop through the
  [bridge in the README](../README.md#connect-your-ai-tutor). It cannot edit
  cards.
- **Cannot connect.** ChatGPT connectors and claude.ai custom connectors call
  MCP servers from the provider's cloud and cannot reach a private Yume.

## Connect Codex or the ChatGPT desktop app

With Yume running on the same machine, register its MCP endpoint:

```sh
codex mcp add yume --url http://127.0.0.1:7654/mcp
```

Install the skill for your user account:

macOS or Linux:

```sh
mkdir -p ~/.agents/skills
cp -R skills/yume-sensei ~/.agents/skills/
```

Windows PowerShell:

```powershell
New-Item -ItemType Directory -Force "$HOME\.agents\skills" | Out-Null
Copy-Item -Recurse skills\yume-sensei "$HOME\.agents\skills\"
```

The Codex CLI, the Codex IDE extension, and Codex in the ChatGPT desktop app
share MCP configuration on the same Codex host. Run `codex mcp list` or open
the app's MCP settings to confirm that Yume is connected. Codex detects skill
changes automatically; restart it if Yume Sensei does not appear.

If Yume runs on another private machine, replace `127.0.0.1` with the same
private address you use to open the app.

## Connect Claude Code

```sh
claude mcp add --transport http yume http://127.0.0.1:7654/mcp
cp -R skills/yume-sensei ~/.claude/skills/
```

Then invoke `/yume-sensei` or ask for help with your Yume deck.

## A normal tutor session

Ask in plain language:

- **“Check my flagged cards.”** The tutor handles the explanation, sentence,
  audio, or correction each card asks for.
- **“Fill the gaps in my deck.”** The tutor finds missing sentences,
  mnemonics, explanations, or audio and works through them in batches.
- **“Suggest words from my gaps in the top frequency bands.”** The tutor adds
  a short list under **Library → Suggested**. Nothing enters review until you
  accept it.
- **“Add kanji stories for the most common kanji in my deck.”** The tutor adds
  reusable notes to the kanji sheets.

Before a large write, ask the tutor to create a vocabulary backup. Review its
summary afterward so you know what changed.

## Request help from a card

Open a card's menu during review and choose **Improve this card**. You can
request:

- an explanation of meaning or nuance
- a natural example sentence
- pronunciation audio
- a correction to the card

The request is a tag stored on the card; it does not contact an AI service.
The next time you ask your tutor to check flagged cards, it sees the tag, does
the work, and clears it.

No tutor? The same sheet has **Write it yourself**: write your own mnemonic
story, add an example sentence with its English, or fix the sentence's
translation and the explanation. These save to the card right away.

## Writing principles

Everything the tutor writes follows [`card-content.md`](card-content.md). In
particular:

- Mnemonics connect the written parts to the meaning in one to three plain
  sentences, followed by a natural usage example.
- Related words already in the deck are useful anchors when they share a kanji.
- Register notes and easily confused words belong only where they help recall
  or prevent a real mistake.
- Example sentences sound like something a person would naturally say.
- Explanations are for genuinely difficult words, not every card.

## Privacy and cost

Yume does not include or bill for an AI model. Usage, data handling, and cost
depend on the AI client and model you connect. Review those terms before
giving a client access to a deck you consider sensitive.

Generated audio is local when you use the optional VOICEVOX workflow. It does
not run inside the Docker image; see
[`server/tools/voicevox`](../server/tools/voicevox/README.md#with-docker) for
running it on the host.
