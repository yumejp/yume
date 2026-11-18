# Security policy

## Supported versions

Yume is in private alpha and has no tagged releases yet. Security fixes land
on `main`, so run the newest `main` or the newest image built from it. Once
releases start, fixes go to the latest release; older releases are not
patched.

## Report a vulnerability

Please do not report security problems in a public issue, pull request or
discussion.

- Use GitHub private vulnerability reporting: open the repository's
  **Security** tab and choose **Report a vulnerability**. It is switched on
  when the repository becomes public.
- Until then, tell the maintainer, [@swei9322](https://github.com/swei9322),
  privately through the channel you were invited by, not in an issue. A
  dedicated security address will be listed here once it exists.

A useful report says:

- the server version (Settings › About in the app, or `/health`) and, if
  you built it yourself, the commit
- how the server was installed (Docker Compose, built from source, Raspberry
  Pi service) and on which platform
- the auth mode (`YUME_AUTH_MODE`) and what the server is bound to
- the steps to reproduce and what an attacker gains

Leave your deck contents, session tokens and private addresses out of the
report unless they are needed to show the problem.

## What to expect

Yume is maintained by one person, so these are targets, not guarantees:

- an acknowledgement within 7 days
- an assessment and a plan within 14 days
- a fix, or a documented workaround, as fast as the severity calls for

You will be told when a fix lands. Say whether you want credit in the release
notes.

## Scope

In scope: `yume-server`, the app it serves, the `/mcp` endpoint, the Docker
image, and documentation that leads people to an unsafe setup.

The server trusts the network it is bound to by design: in the default open
auth mode, anyone who can reach the port has the owner's full access. A report
that open mode gives such access is expected behaviour, unless the server
does so while bound to loopback, or a browser page can reach it across the
Host and Origin checks.
[docs/security.md](https://github.com/yumejp/yume/blob/main/docs/security.md)
describes the model.

Dictionary data (JMdict, KANJIDIC2, Tatoeba, Wiktionary) and VOICEVOX belong
to their own projects; report problems in them upstream.
