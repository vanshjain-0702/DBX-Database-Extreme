#!/usr/bin/env python3
"""Narrated recall / Updates clip: AndrewNeural + 140 BPM bed.

Reuses helpers from produce_demo_video.py. Raw capture from RecordScreen.
"""

from __future__ import annotations

import asyncio
import os
import shutil
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import produce_demo_video as prod  # noqa: E402

RAW = Path("/opt/cursor/artifacts/dbx-recall-raw.mp4")
OUT = Path("/workspace/website/assets/vector-search.mp4")
ARTIFACT = Path("/opt/cursor/artifacts/dbx-recall-walkthrough.mp4")

SCRIPT: list[tuple[str | float, str]] = [
    (
        "intro",
        "Hey — this is the recall cut. What shipped: VSEARCH, VSIM, and VFUSE "
        "on one tenant, plus the site benches so you can feel the product without a live node. "
        "Embeddings stay with you. We do not run CLIP or MiniLM.",
    ),
    (
        "home",
        "Homepage. New post in the strip, unread badge on Updates. "
        "That's the notify — no mail list, hello at dbxdb.io still has no MX.",
    ),
    (
        "leak",
        "Prefix leak versus a tenant worker. Left side: KEYS user star on a shared cluster "
        "returns alice and bob. Right side: AUTH switches process. GET on bob cannot see alice. "
        "Browser sketch — not a live node.",
    ),
    (
        "recall",
        "Toy cosine on six vectors. VSIM alpha excludes self. SPACE image keeps text neighbors out. "
        "VFUSE is a weighted sum of two spaces, not a multimodal model. "
        "MIN_SCORE drops the weak hits. Certified ANN p fifty is two point three zero four milliseconds — not sub-millisecond.",
    ),
    (
        "density",
        "Idle density is RSS, not a price. Strict Isolation Kernel workers sit fourteen to seventeen mebibytes. "
        "Slider is midpoint times tenant count. No invented dollars.",
    ),
    (
        "quiz",
        "Four-question fit check. If you need SQL joins or CLIP in-process, walk away. "
        "If tenant is the unit and you send the floats, this is the product.",
    ),
    (
        "posts",
        "Updates page. Recall post, Isolation Kernel, walkthrough, v1. Opening it clears the badge. "
        "RSS is posts.xml if you want a feed without giving us an inbox.",
    ),
    (
        "shred",
        "Security: revoke the D E K. WAL, meta, and the graph go unreadable. "
        "SQ8 vec rows stay mmap plaintext unless the volume is LUKS or fscrypt. That's the honest limit.",
    ),
    (
        "close",
        "License is B S L 1.1. Self-host is free, including inside your own SaaS. "
        "That's the recall upgrade.",
    ),
]


async def main() -> None:
    if not RAW.exists():
        raise SystemExit(f"missing raw capture {RAW}")
    work = Path(tempfile.mkdtemp(prefix="dbx-recall-"))
    raw_dur = prod.duration(RAW)
    keep = max(8.0, raw_dur - prod.CURSOR_TRIM)
    intro = work / "intro.mp4"
    outro = work / "outro.mp4"
    prod.card(
        intro,
        "RECALL UPGRADE",
        "VSEARCH · VSIM · VFUSE",
        "One tenant. Embeddings stay with the caller.",
        7.5,
    )
    prod.card(outro, "BSL 1.1", "Self-host it.", "Free inside your own SaaS", 6.0)
    body = work / "body.mp4"
    prod.run(
        [
            "ffmpeg",
            "-y",
            "-i",
            str(RAW),
            "-t",
            f"{keep:.3f}",
            "-an",
            *prod.x264_cfr(),
            str(body),
        ]
    )
    picture = work / "picture.mp4"
    prod.concat_videos([intro, body, outro], picture)
    intro_d = prod.duration(intro)
    pic_dur = prod.duration(picture)
    print("picture", pic_dur)

    marks = {
        "intro": 0.5,
        "home": intro_d + 1.0,
        "leak": intro_d + min(18.0, keep * 0.12),
        "recall": intro_d + min(38.0, keep * 0.28),
        "density": intro_d + min(58.0, keep * 0.42),
        "quiz": intro_d + min(72.0, keep * 0.52),
        "posts": intro_d + min(88.0, keep * 0.64),
        "shred": intro_d + min(104.0, keep * 0.76),
        "close": intro_d + keep + 0.4,
    }

    clips: list[tuple[float, Path, float]] = []
    cursor = 0.35
    for i, (key, text) in enumerate(SCRIPT):
        wav = work / f"n{i:02d}.wav"
        print(f"tts {i+1}/{len(SCRIPT)}")
        await prod.synth(text, wav)
        dur = prod.duration(wav)
        start = max(float(marks.get(key, cursor)), cursor)
        clips.append((start, wav, dur))
        cursor = start + dur + 0.5
        print(f"  @{start:.1f}s  {dur:.1f}s")

    last_end = max(s + d for s, _, d in clips) + 0.6
    if last_end > pic_dur + 0.05:
        extra = last_end - pic_dur
        print(f"== pad picture {extra:.1f}s")
        padded = work / "picture_pad.mp4"
        prod.pad_picture(picture, padded, extra)
        picture = padded
        pic_dur = prod.duration(picture)

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
    prod.run(
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
    prod.make_music(music, pic_dur)
    mixed = work / "mixed.wav"
    prod.run(
        [
            "ffmpeg",
            "-y",
            "-i",
            str(voice),
            "-i",
            str(music),
            "-filter_complex",
            "[0]asplit=2[vx][sc];"
            "[1]volume=0.38,highpass=f=90,lowpass=f=3200[mb];"
            "[mb][sc]sidechaincompress=threshold=0.022:ratio=10:attack=18:"
            "release=280:level_sc=1:makeup=1[md];"
            "[vx]volume=1.05[vv];"
            "[vv][md]amix=inputs=2:duration=first:dropout_transition=2:normalize=0,"
            "alimiter=limit=0.94",
            str(mixed),
        ]
    )
    staged = work / "out.mp4"
    prod.run(
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
            "title=DBX recall upgrade",
            "-metadata",
            "comment=VSEARCH VSIM VFUSE walkthrough",
            str(staged),
        ]
    )
    shutil.copyfile(staged, OUT)
    shutil.copyfile(staged, ARTIFACT)
    print("wrote", OUT, f"{OUT.stat().st_size/1e6:.1f}MB", "dur", prod.duration(OUT))
    shutil.rmtree(work, ignore_errors=True)


if __name__ == "__main__":
    os.environ.setdefault("PYTHONUNBUFFERED", "1")
    asyncio.run(main())
