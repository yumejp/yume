# Third-party data & software notices

Yume bundles or loads the following third-party dictionaries, data files, and
libraries. This file collects the attribution and notice obligations their
licenses require. If you redistribute Yume (or a modified version), keep this
file with it.

---

## JMdict (Japanese–English dictionary data)

This application uses the **JMdict** dictionary file. JMdict is the property
of the **Electronic Dictionary Research and Development Group** (EDRDG), and
is used in conformance with the Group's
[licence](https://www.edrdg.org/edrdg/licence.html).

- JMdict is © the Electronic Dictionary Research and Development Group,
  licensed under
  [Creative Commons Attribution-ShareAlike 4.0 International](https://creativecommons.org/licenses/by-sa/4.0/)
  (CC BY-SA 4.0).
- Project page: <https://www.edrdg.org/jmdict/j_jmdict.html>
- Yume loads `JMdict_e.gz` at runtime (it is not embedded in the binary). The
  app credits "JMdict © EDRDG, CC BY-SA 4.0" under dictionary results and in
  the word popup, and its About sheet links to the EDRDG license, the project
  pages and these notices.
- EDRDG updates JMdict regularly. The README's "Refresh the dictionary files"
  section is the update procedure.

## IPADIC (Japanese morphological dictionary, embedded via kagome-dict/ipa)

The tokenizer embeds dictionary data derived from
**mecab-ipadic-2.7.0-20070801** (via
[`github.com/ikawaha/kagome-dict/ipa`](https://github.com/ikawaha/kagome-dict)).
The IPADIC notice below must accompany redistribution:

> Nara Institute of Science and Technology (NAIST), the copyright holders,
> disclaims all warranties with regard to this software, including all
> implied warranties of merchantability and fitness, in no event shall NAIST
> be liable for any special, indirect or consequential damages or any
> damages whatsoever resulting from loss of use, data or profits, whether in
> an action of contract, negligence or other tortuous action, arising out of
> or in connection with the use or performance of this software.
>
> A large portion of the dictionary entries originate from ICOT Free
> Software. The following conditions for ICOT Free Software applies to the
> current dictionary as well.
>
> Each User may also freely distribute the Program, whether in its original
> form or modified, to any third party or parties, PROVIDED that the
> provisions of Section 3 ("NO WARRANTY") will ALWAYS appear on, or be
> attached to, the Program, which is distributed substantially in the same
> form as set out herein and that such intended distribution, if actually
> made, will neither violate or otherwise contravene any of the laws and
> regulations of the countries having jurisdiction over the User or the
> intended distribution itself.
>
> **NO WARRANTY**
>
> The program was produced on an experimental basis in the course of the
> research and development conducted during the project and is provided to
> users as so produced on an experimental basis. Accordingly, the program is
> provided without any warranty whatsoever, whether express, implied,
> statutory or otherwise. The term "warranty" used herein includes, but is
> not limited to, any warranty of the quality, performance, merchantability
> and fitness for a particular purpose of the program and the nonexistence
> of any infringement or violation of any right of any third party.
>
> Each user of the program will agree and understand, and be deemed to have
> agreed and understood, that there is no warranty whatsoever for the
> program and, accordingly, the entire risk arising from or otherwise
> connected with the program is assumed by the user.
>
> Therefore, neither ICOT, the copyright holder, or any other organization
> that participated in or was otherwise related to the development of the
> program and their respective officials, directors, officers and other
> employees shall be held liable for any and all damages, including, without
> limitation, general, special, incidental and consequential damages,
> arising out of or otherwise in connection with the use or inability to use
> the program or any product, material or result produced or otherwise
> obtained by using the program, regardless of whether they have been
> advised of, or otherwise had knowledge of, the possibility of such damages
> at any time during the project or thereafter. Each user will be deemed to
> have agreed to the foregoing by his or her commencement of use of the
> program. The term "use" as used herein includes, but is not limited to,
> the use, modification, copying and distribution of the program and the
> production of secondary products from the program.
>
> In the case where the program, whether in its original form or modified,
> was distributed or delivered to or received by a user from any person,
> organization or entity other than ICOT, unless it makes or grants
> independently of ICOT any specific warranty to the user in writing, such
> person, organization or entity, will also be exempted from and not be held
> liable to the user for any such damages as noted above as far as the
> program is concerned.

## KANJIDIC2 (kanji dictionary data)

Per-kanji meanings, readings, grade, JLPT level, and stroke counts come from
the **KANJIDIC2** file, the property of the **Electronic Dictionary Research
and Development Group** (EDRDG), used in conformance with the Group's
[licence](https://www.edrdg.org/edrdg/licence.html).

- KANJIDIC2 is © EDRDG, licensed under
  [CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/).
- Project page: <https://www.edrdg.org/wiki/index.php/KANJIDIC_Project>
- Yume loads `kanjidic2.xml.gz` at runtime and credits
  "KANJIDIC2 / EDRDG" where the data is shown.

## Tanaka Corpus / Tatoeba example sentences

Example sentences come from the **Tanaka Corpus** example file
(`examples.utf.gz`) as maintained by the
[Tatoeba project](https://tatoeba.org) and distributed by EDRDG.

- Licensed under
  [Creative Commons Attribution 2.0 France](https://creativecommons.org/licenses/by/2.0/fr/)
  (CC BY 2.0 FR), per Tatoeba's terms of use.
- Project page: <https://www.edrdg.org/wiki/index.php/Tanaka_Corpus>
- Yume loads the file at runtime (not embedded) and credits "Tanaka Corpus
  via Tatoeba, CC BY 2.0 FR" under corpus example sentences in the dictionary,
  the word popup and the review card.

## User-imported dictionaries

Yomitan-format dictionaries (term/pitch/frequency) imported by the user are
subject to their own licenses; importing them does not transfer any rights
to Yume. Check each dictionary's license before redistribution.

## Japanese Wiktionary (optional Japanese-Japanese dictionary)

`yume-server dict ja-ja` (or `YUME_DICTS=ja-ja`) downloads a Yomitan-format
dictionary built from the **Japanese Wiktionary** by
[Kaikki-to-Yomitan](https://github.com/yomidevs/kaikki-to-yomitan) (via
[kaikki.org](https://kaikki.org/)). Wiktionary text is licensed under
[CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/); the
converter is open source. The data is fetched at install time, not bundled.
The app credits it in Settings › About when installed.

## Libraries

| Library | License |
|---|---|
| [kagome v2](https://github.com/ikawaha/kagome) (tokenizer) | MIT |
| [kagome-dict / kagome-dict/ipa](https://github.com/ikawaha/kagome-dict) (wrapper code) | MIT (embedded data: IPADIC notice above) |
| [go-fsrs](https://github.com/open-spaced-repetition/go-fsrs) (SRS scheduling) | MIT |
| [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) (pure-Go SQLite) | BSD-3-Clause |
| [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk) (the /mcp endpoint) | Apache-2.0 (earlier contributions MIT) |
| [golang.org/x/crypto](https://pkg.go.dev/golang.org/x/crypto) (password hashing) | BSD-3-Clause |
| [React](https://react.dev) | MIT |
| [wanakana](https://wanakana.com) (romaji↔kana) | MIT |
| [Expo](https://expo.dev) + [React Native](https://reactnative.dev) (app) | MIT |
| [react-native-svg](https://github.com/software-mansion/react-native-svg) (app) | MIT |

These are the main components, not the full list. The license text of every
Go module linked into `yume-server` and every production npm package of the
bundled app is in
[`server/internal/notices/THIRD_PARTY_NOTICES.txt`](server/internal/notices/THIRD_PARTY_NOTICES.txt),
which begins with this file. A running server serves it at `/licenses` (linked
from Settings › About), and the Docker image ships it and this file under
`/usr/share/doc/yume/`. `node scripts/third-party-notices.mjs` regenerates it;
CI fails when it is stale or when a dependency's license is GPL-family, SSPL,
non-commercial or unknown.

`node-forge` (pulled in by Expo's build tooling) is offered under
BSD-3-Clause OR GPL-2.0; Yume uses it under BSD-3-Clause.
