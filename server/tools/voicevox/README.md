# Card audio with VoiceVox

`yume-server voicevox` puts the engine, voice models and a Python environment
under `<data>/voicevox` and copies `batch-audio.py` next to them. It needs
Python 3.10 or later. Then, on macOS or Linux:

```sh
<data>/voicevox/venv/bin/python <data>/voicevox/batch-audio.py
```

On Windows:

```powershell
<data>\voicevox\venv\Scripts\python.exe <data>\voicevox\batch-audio.py
```

The script voices every card that has no audio yet (two clips: the word and
the sentence) and records the voice name on each clip, which is the credit
VOICEVOX's terms ask for. It takes about 10 seconds per card on a Raspberry
Pi 4, much less on a laptop, so a large import can take hours. Stop it any
time and re-run it later; it only touches cards still missing audio.

Change the voice with `VOICE_STYLES` (one style id, or several to spread
cards across voices; the credit comes from the model file). Point it at a
remote server with `YUME_URL`, and `YUME_TOKEN` when accounts are on.

## With Docker

`docker compose exec yume yume-server voicevox` does not work: the image is
Alpine Linux without Python, VOICEVOX's Python package needs glibc, and this
folder is not copied into the image. Run the audio step on the host instead,
from a checkout of this repository, with [Go 1.26+](https://go.dev/dl/) and
Python 3.10+ installed:

```sh
cd server
go run . voicevox -data ../voicevox-host
../voicevox-host/voicevox/venv/bin/python ../voicevox-host/voicevox/batch-audio.py
```

On Windows, run the last step as
`..\voicevox-host\voicevox\venv\Scripts\python.exe ..\voicevox-host\voicevox\batch-audio.py`.

`-data` only chooses where VOICEVOX is installed; your deck stays in the
container. The script talks to the container at `http://127.0.0.1:7654`. If
you bound Yume to another address with `YUME_BIND`, set `YUME_URL` to it, for
example `YUME_URL=http://100.101.102.103:7654`.
