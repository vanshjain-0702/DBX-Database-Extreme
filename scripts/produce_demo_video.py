#!/usr/bin/env python3
"""Produce a calm, professional DBX walkthrough from a screen capture.

SDE pairing-session VO (en-US-AndrewNeural): contractions, ports, fail-closed.
Audible pentatonic bed (laptop-speaker range), ducked under speech.
Chapter cards. Cursor end-tag trimmed.
"""

from __future__ import annotations

import asyncio
import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

import edge_tts

RAW = Path("/opt/cursor/artifacts/dbx-raw-walkthrough.mp4")
MARKS = Path("/tmp/dbx-demo/marks.txt")
OUT = Path("/workspace/website/assets/demo.mp4")
ARTIFACT = Path("/opt/cursor/artifacts/dbx-product-walkthrough.mp4")
NARRATED = Path("/opt/cursor/artifacts/dbx-product-walkthrough-narrated.mp4")
FONT = "/usr/share/fonts/truetype/noto/NotoSans-Bold.ttf"
FONT2 = "/usr/share/fonts/truetype/noto/NotoSans-Regular.ttf"
VOICE = "en-US-AndrewNeural"
RATE = "-6%"
PITCH = "-1Hz"
# Drop Cursor brand (~2s) from a finished capture; producer also trims via -sseof.
CURSOR_TRIM = 2.05

# Engineer walking a teammate through the product. Contractions, concrete ports, no marketing.
SCRIPT: list[tuple[str | float, str]] = [
    (
        "intro",
        "Hey — I'm walking you through DBX the way I'd walk a teammate through it. "
        "It's a per-tenant memory engine: strings and vectors, one isolated engine per customer. "
        "Site first, then we log into the dashboard and actually provision a tenant.",
    ),
    (
        "site_home",
        "Okay, homepage. The claim is structural isolation, not a Redis-style key prefix. "
        "Each tenant is a directory, a write-ahead log, and an H N S W index, same process for KV and vectors. "
        "If you forget a prefix in a shared cluster, you leak data. That failure mode is what we're avoiding.",
    ),
    (
        "bench",
        "This widget is a browser sketch. Don't treat it as a live node. "
        "Three engines: acme, harbor, lumen. I'll AUTH harbor, SET a session, AUTH acme, GET. "
        "Acme comes back empty. That's the invariant — GET cannot cross a tenant boundary.",
    ),
    (
        "site_why",
        "If you've operated multi-tenant Redis, you already know this: tenancy as a prefix, "
        "session in one store, embeddings in another, backup is the whole box. "
        "We made the tenant the unit instead. Provision, backup, and delete operate on one customer.",
    ),
    (
        "site_pages",
        "Command palette is Control K. Five things have to stay true in the code: "
        "tenant is first-class, KV plus vectors in one engine, cost tracks active tenants, "
        "single self-hosted binary, and Isolation Kernel on Linux strict. "
        "Everything else is in service of that.",
    ),
    (
        "start",
        "Local run is boring on purpose. Dashboard on eight thousand, public RESP on six three eight zero. "
        "Docker, source, or Compose. Mint a writer key, then AUTH tenant I D, colon, key I D, space, secret. "
        "That's the data plane. The dashboard console is operator JWT — different path.",
    ),
    (
        "perf",
        "These are single-node, single-tenant benches. "
        "About one lakh eighty-six thousand SETs a second, recall at ten of zero point nine two. "
        "I wouldn't quote them in a design doc until you've reproduced them on your hardware.",
    ),
    (
        "docs",
        "Lifecycle you actually call: provision, backup, restore, hibernate, wake. "
        "v1 durable commands are strings and vectors — SET, GET, VADD, VSEARCH. "
        "Hashes, lists, Raft, cluster: fail closed. We don't stub them and hope.",
    ),
    (
        "security",
        "Strict mode is the security USP: own process, Landlock, encrypted WAL. "
        "make run-dev is in-process. Don't ship that as Isolation Kernel. "
        "Mail isn't wired yet — file a GitHub issue.",
    ),
    (
        "part2",
        "Dashboard is compiled into the orchestrator. It's not on GitHub Pages. "
        "We're going to log in and drive one tenant end to end.",
    ),
    (
        "dash_login",
        "Dev login is admin. Then provision. Name Demo Acme, I D demo-acme, replicas none. "
        "That's the certified single-node path — don't turn replicas on for the happy path.",
    ),
    (
        "dash_overview",
        "Overview is this engine's live RSS and ops, not a fake Ready badge. "
        "Backup here archives this tenant. It's not a VM snapshot of the node.",
    ),
    (
        "dash_keys",
        "Mint a writer. Your app AUTHs on six three eight zero with tenant I D, key I D, and the secret. "
        "Secret is shown once — copy it. Reader keys cannot SET or VADD. That's enforced.",
    ),
    (
        "dash_console",
        "Console is the operator token posting to slash t slash id slash query. "
        "PING, SET session forty-two, GET. Same process you just provisioned. "
        "If this returns onboarding, the engine is actually up.",
    ),
    (
        "dash_explorer",
        "Explorer is KEYS and GET with a JSON preview. "
        "You should see session forty-two from the console write. You can SET another string from here too.",
    ),
    (
        "dash_vector",
        "VADD writes a vector into that tenant's index. VSEARCH is nearest neighbour. "
        "Playground can embed in the browser with MiniLM — I'm skipping the model download. "
        "Use the console VADD if you just need to prove the index path.",
    ),
    (
        "runtime",
        "Hardware, storage, network, hosting: process telemetry. "
        "Settings and replication are control plane. Replication is async WAL, primary acks locally. "
        "It is not Raft. Don't design failover as if it were.",
    ),
    (
        "close",
        "License is B S L 1.1. Self-host is free, including inside your own SaaS. "
        "You pay only if DBX itself is the product you sell. That's the whole commercial split. "
        "Alright — that's the tour.",
    ),
]


def run(cmd: list[str], **kw) -> None:
    subprocess.check_call(cmd, **kw)


def duration(path: Path) -> float:
    out = subprocess.check_output(
        [
            "ffprobe",
            "-v",
            "error",
            "-show_entries",
            "format=duration",
            "-of",
            "default=noprint_wrappers=1:nokey=1",
            str(path),
        ],
        text=True,
    ).strip()
    return float(out)


def parse_marks() -> dict[str, float]:
    found: dict[str, float] = {}
    if MARKS.exists():
        for line in MARKS.read_text().splitlines():
            parts = line.strip().split(maxsplit=1)
            if len(parts) == 2:
                try:
                    found[parts[1].strip()] = float(parts[0])
                except ValueError:
                    continue
    return found


def card(path: Path, kicker: str, title: str, sub: str, seconds: float = 7.0) -> None:
    def esc(s: str) -> str:
        return s.replace("\\", "\\\\").replace(":", "\\:").replace("'", "\\'")

    run(
        [
            "ffmpeg",
            "-y",
            "-f",
            "lavfi",
            "-i",
            f"color=c=0x040910:s=1920x1200:d={seconds}:r=30",
            "-vf",
            (
                f"drawtext=fontfile={FONT2}:text='{esc(kicker)}':fontcolor=0x8ea8bd:"
                f"fontsize=28:x=(w-text_w)/2:y=h/2-110,"
                f"drawtext=fontfile={FONT}:text='{esc(title)}':fontcolor=0xf1f6fb:"
                f"fontsize=64:x=(w-text_w)/2:y=h/2-40,"
                f"drawtext=fontfile={FONT2}:text='{esc(sub)}':fontcolor=0x8ea8bd:"
                f"fontsize=28:x=(w-text_w)/2:y=h/2+48"
            ),
            "-c:v",
            "libx264",
            "-pix_fmt",
            "yuv420p",
            "-preset",
            "fast",
            "-crf",
            "18",
            str(path),
        ]
    )


def make_music(path: Path, seconds: float) -> None:
    """Warm pad + C-major pentatonic ostinato in the 400–1200 Hz laptop band."""
    work = path.parent
    pad = work / "pad.wav"
    run(
        [
            "ffmpeg",
            "-y",
            "-f",
            "lavfi",
            "-i",
            f"sine=frequency=130.81:duration={seconds+2}",
            "-f",
            "lavfi",
            "-i",
            f"sine=frequency=196.00:duration={seconds+2}",
            "-f",
            "lavfi",
            "-i",
            f"sine=frequency=261.63:duration={seconds+2}",
            "-f",
            "lavfi",
            "-i",
            f"sine=frequency=329.63:duration={seconds+2}",
            "-filter_complex",
            "[0]volume=0.16[a];[1]volume=0.14[b];[2]volume=0.11[c];[3]volume=0.08[d];"
            "[a][b][c][d]amix=inputs=4:duration=longest:normalize=0,"
            "lowpass=f=900,tremolo=f=0.12:d=0.22,volume=0.85",
            str(pad),
        ],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    # One phrase (~8.2s), then loop. Melody sits on C5 so laptop speakers hear a tune.
    notes = [
        (523.25, 0.70),
        (659.25, 0.70),
        (783.99, 1.05),
        (659.25, 0.70),
        (587.33, 0.70),
        (523.25, 1.20),
        (0.0, 0.40),
        (440.00, 0.70),
        (523.25, 0.70),
        (659.25, 1.05),
        (523.25, 0.70),
        (493.88, 0.70),
        (440.00, 1.20),
        (0.0, 0.40),
    ]
    segs = []
    for i, (freq, dur) in enumerate(notes):
        seg = work / f"n{i}.wav"
        if freq <= 0:
            run(
                [
                    "ffmpeg",
                    "-y",
                    "-f",
                    "lavfi",
                    "-i",
                    f"anullsrc=r=44100:cl=stereo:d={dur}",
                    str(seg),
                ],
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
            )
        else:
            run(
                [
                    "ffmpeg",
                    "-y",
                    "-f",
                    "lavfi",
                    "-i",
                    f"sine=frequency={freq}:duration={dur}:sample_rate=44100",
                    "-f",
                    "lavfi",
                    "-i",
                    f"sine=frequency={freq*2}:duration={dur}:sample_rate=44100",
                    "-filter_complex",
                    "[0]volume=0.70[a];[1]volume=0.16[b];"
                    "[a][b]amix=inputs=2:duration=longest:normalize=0,"
                    f"afade=t=in:d=0.04,afade=t=out:st={max(0.08, dur-0.28)}:d=0.28,"
                    "volume=1.0",
                    "-ac",
                    "2",
                    str(seg),
                ],
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
            )
        segs.append(seg)
    lst = work / "melody.txt"
    lst.write_text("".join(f"file '{p}'\n" for p in segs))
    cycle = work / "melody_cycle.wav"
    run(
        [
            "ffmpeg",
            "-y",
            "-f",
            "concat",
            "-safe",
            "0",
            "-i",
            str(lst),
            "-c",
            "copy",
            str(cycle),
        ],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    melody = work / "melody.wav"
    run(
        [
            "ffmpeg",
            "-y",
            "-stream_loop",
            "-1",
            "-i",
            str(cycle),
            "-t",
            str(seconds + 1),
            str(melody),
        ],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    run(
        [
            "ffmpeg",
            "-y",
            "-i",
            str(pad),
            "-i",
            str(melody),
            "-filter_complex",
            "[0]volume=0.50[p];[1]volume=0.95,highpass=f=280,lowpass=f=2400[m];"
            "[p][m]amix=inputs=2:duration=first:normalize=0,"
            f"alimiter=limit=0.55,afade=t=in:d=1.6,afade=t=out:st={max(1.0, seconds-3.5)}:d=3.5",
            "-t",
            str(seconds),
            str(path),
        ],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )


async def synth(text: str, dest: Path) -> None:
    tmp = dest.with_suffix(".mp3")
    await edge_tts.Communicate(text, VOICE, rate=RATE, pitch=PITCH).save(str(tmp))
    run(
        [
            "ffmpeg",
            "-y",
            "-i",
            str(tmp),
            "-ac",
            "2",
            "-ar",
            "44100",
            str(dest),
        ],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    tmp.unlink(missing_ok=True)


async def main() -> None:
    audio_only = "--audio-only" in sys.argv
    if not RAW.exists():
        raise SystemExit(f"missing raw capture {RAW}")
    if audio_only and not OUT.exists():
        raise SystemExit("audio-only needs an existing website/assets/demo.mp4")
    work = Path(tempfile.mkdtemp(prefix="dbx-prod-"))
    marks = parse_marks()
    raw_dur = duration(RAW)
    keep = max(8.0, raw_dur - CURSOR_TRIM)
    if "close" in marks:
        trailing = 7.0
        offset = max(0.0, keep - (marks["close"] + trailing))
        marks = {k: v + offset for k, v in marks.items()}
        print("align offset", round(offset, 2))
    print("marks", {k: round(v, 1) for k, v in marks.items()})

    dash_at = marks.get("dash_login", keep * 0.55)
    site_len = max(4.0, dash_at - 0.15)
    picture = work / "picture.mp4"

    if audio_only:
        print("== reuse existing picture")
        run(
            [
                "ffmpeg",
                "-y",
                "-i",
                str(OUT),
                "-an",
                "-c:v",
                "copy",
                str(picture),
            ]
        )
        intro_d = 8.5
        part2_d = 6.5
        pic_dur = duration(picture)
        print("picture (reused)", pic_dur)
    else:
        intro = work / "intro.mp4"
        part2 = work / "part2.mp4"
        outro = work / "outro.mp4"
        card(
            intro,
            "PRODUCT WALKTHROUGH",
            "DBX",
            "Per-tenant memory for AI products",
            8.5,
        )
        card(
            part2,
            "PART TWO",
            "Operator dashboard",
            "Provision, keys, console, vectors",
            6.5,
        )
        card(outro, "BSL 1.1", "Self-host it.", "Free inside your own SaaS", 7.0)

        body = work / "body.mp4"
        run(
            [
                "ffmpeg",
                "-y",
                "-i",
                str(RAW),
                "-t",
                f"{keep:.3f}",
                "-an",
                "-c:v",
                "libx264",
                "-preset",
                "fast",
                "-crf",
                "19",
                "-pix_fmt",
                "yuv420p",
                "-r",
                "30",
                str(body),
            ]
        )

        site = work / "site.mp4"
        dash = work / "dash.mp4"
        run(
            [
                "ffmpeg",
                "-y",
                "-i",
                str(body),
                "-t",
                f"{site_len:.3f}",
                "-c",
                "copy",
                str(site),
            ]
        )
        run(
            [
                "ffmpeg",
                "-y",
                "-ss",
                f"{site_len:.3f}",
                "-i",
                str(body),
                "-c",
                "copy",
                str(dash),
            ]
        )

        lst = work / "concat.txt"
        pieces = [intro, site, part2, dash, outro]
        lst.write_text("".join(f"file '{p}'\n" for p in pieces))
        run(
            [
                "ffmpeg",
                "-y",
                "-f",
                "concat",
                "-safe",
                "0",
                "-i",
                str(lst),
                "-c",
                "copy",
                str(picture),
            ]
        )
        intro_d = duration(intro)
        part2_d = duration(part2)
        pic_dur = duration(picture)
        print("picture", pic_dur)

    shift = {"intro": 0.6, "part2": intro_d + site_len + 0.4}

    def abs_t(key: str | float) -> float:
        if isinstance(key, (int, float)):
            return float(key)
        if key in shift:
            return shift[key]
        if key in marks:
            t = marks[key]
            if t >= site_len:
                return intro_d + site_len + part2_d + (t - site_len)
            return intro_d + t
        return intro_d + 1.0

    clips: list[tuple[float, Path, float]] = []
    cursor = 0.4
    for i, (key, text) in enumerate(SCRIPT):
        wav = work / f"n{i:02d}.wav"
        print(f"tts {i+1}/{len(SCRIPT)}")
        await synth(text, wav)
        dur = duration(wav)
        start = max(abs_t(key), cursor)
        if start + dur > pic_dur - 0.3:
            start = max(0.2, pic_dur - dur - 0.4)
        clips.append((start, wav, dur))
        cursor = start + dur + 0.9
        print(f"  @{start:.1f}s  {dur:.1f}s")

    inputs: list[str] = []
    filters = []
    for i, (start, wav, _) in enumerate(clips):
        inputs += ["-i", str(wav)]
        ms = int(round(start * 1000))
        filters.append(f"[{i}]adelay={ms}|{ms}[v{i}]")
    mix_in = "".join(f"[v{i}]" for i in range(len(clips)))
    filters.append(
        f"{mix_in}amix=inputs={len(clips)}:duration=longest:dropout_transition=0:normalize=0,"
        f"apad=whole_dur={pic_dur:.3f},atrim=0:{pic_dur:.3f},asetpts=PTS-STARTPTS[voice]"
    )
    voice = work / "voice.wav"
    run(
        [
            "ffmpeg",
            "-y",
            *inputs,
            "-filter_complex",
            ";".join(filters),
            "-map",
            "[voice]",
            str(voice),
        ]
    )

    music = work / "music.wav"
    make_music(music, pic_dur)
    mixed = work / "mixed.wav"
    run(
        [
            "ffmpeg",
            "-y",
            "-i",
            str(voice),
            "-i",
            str(music),
            "-filter_complex",
            # Music stays in the 300–2 kHz band. Duck under speech so the
            # ostinato is obvious in gaps without talking over the VO.
            "[0]asplit=2[vx][sc];"
            "[1]volume=0.72,highpass=f=90,lowpass=f=2600[mb];"
            "[mb][sc]sidechaincompress=threshold=0.028:ratio=7:attack=25:"
            "release=380:level_sc=1:makeup=1[md];"
            "[vx]volume=1.02[vv];"
            "[vv][md]amix=inputs=2:duration=first:dropout_transition=2:normalize=0,"
            "alimiter=limit=0.94",
            str(mixed),
        ]
    )

    staged = work / "out.mp4"
    run(
        [
            "ffmpeg",
            "-y",
            "-i",
            str(picture),
            "-i",
            str(mixed),
            "-c:v",
            "copy",
            "-c:a",
            "aac",
            "-b:a",
            "192k",
            "-ar",
            "44100",
            "-t",
            f"{pic_dur:.3f}",
            "-movflags",
            "+faststart",
            "-map_metadata",
            "-1",
            "-metadata",
            "title=DBX product walkthrough",
            "-metadata",
            "comment=DBX product walkthrough",
            str(staged),
        ]
    )
    shutil.copyfile(staged, OUT)
    shutil.copyfile(staged, ARTIFACT)
    shutil.copyfile(staged, NARRATED)
    print("wrote", OUT, f"{OUT.stat().st_size/1e6:.1f}MB", "dur", duration(OUT))
    shutil.rmtree(work, ignore_errors=True)


if __name__ == "__main__":
    os.environ.setdefault("PYTHONUNBUFFERED", "1")
    asyncio.run(main())
