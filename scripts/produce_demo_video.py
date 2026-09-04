#!/usr/bin/env python3
"""Produce a calm, professional DBX walkthrough from a screen capture.

Male English voice (en-GB-RyanNeural): slow, composed, widely understood.
Original low chord pad — not a stock track. Chapter cards. Cursor end-tag trimmed.
"""

from __future__ import annotations

import asyncio
import os
import shutil
import subprocess
import tempfile
from pathlib import Path

import edge_tts

RAW = Path("/opt/cursor/artifacts/dbx-raw-walkthrough.mp4")
MARKS = Path("/tmp/dbx-demo/marks.txt")
OUT = Path("/workspace/website/assets/demo.mp4")
ARTIFACT = Path("/opt/cursor/artifacts/dbx-product-walkthrough.mp4")
FONT = "/usr/share/fonts/truetype/noto/NotoSans-Bold.ttf"
FONT2 = "/usr/share/fonts/truetype/noto/NotoSans-Regular.ttf"
VOICE = "en-GB-RyanNeural"
RATE = "-15%"
PITCH = "-3Hz"
# Drop Cursor brand (~2s) from a finished capture; producer also trims via -sseof.
CURSOR_TRIM = 2.05

# (mark_name or absolute seconds, spoken line)
# Lines are short and explanatory. The mixer never speeds them up.
SCRIPT: list[tuple[str | float, str]] = [
    (
        "intro",
        "Welcome to DBX. DBX is a per-tenant memory engine for AI products. "
        "We will go slowly. First, the public website. Then the operator dashboard, "
        "where we will provision a tenant and run real commands.",
    ),
    (
        "site_home",
        "This is the home page. The idea is simple. One isolated engine per customer — "
        "not a shared cluster with a name prefix. Working state and vector memory live "
        "in the same process. Isolation is a directory, a write-ahead log, and an H N S W index.",
    ),
    (
        "bench",
        "On the right is the isolation bench. It is only a sketch in the browser, not a live node. "
        "acme, harbor, and lumen are three separate engines. "
        "AUTH to harbor, SET a session, then AUTH to acme and GET. "
        "Acme cannot see harbor's data. That is the whole product, in one picture.",
    ),
    (
        "site_why",
        "Shared clusters make tenancy your job. If the tenant is only a key prefix, "
        "one missed prefix is a cross-customer leak. Backup and delete become cluster-wide. "
        "In DBX, the tenant is the unit of everything.",
    ),
    (
        "site_pages",
        "Use Control K to move across the site. Five claims have to stay true in the code: "
        "first-class tenants, state and vectors together, cost that follows active tenants, "
        "one self-hosted binary, and an Isolation Kernel on Linux strict mode.",
    ),
    (
        "start",
        "To run it locally: the dashboard is on port eight thousand. "
        "Public RESP is on port six three eight zero. "
        "Install with Docker, from source, or Compose. "
        "After you mint a key, the first data-plane command is AUTH — tenant I D, key I D, and secret.",
    ),
    (
        "perf",
        "These numbers are certified on one node, one tenant. "
        "About one lakh eighty-six thousand SET operations per second. "
        "Recall at ten is zero point nine two. Please measure on your own hardware before you rely on them.",
    ),
    (
        "docs",
        "The docs list the full tenant life: provision, backup, restore, hibernate, and wake. "
        "Durable commands in version one are strings and vectors — SET, GET, VADD, VSEARCH. "
        "Raft and cluster mode fail closed.",
    ),
    (
        "security",
        "Isolation Kernel means own process, Landlock, and an encrypted log. "
        "Default make run-dev is in-process. That is not the security claim. "
        "Mail is not live yet, so use GitHub issues.",
    ),
    (
        "part2",
        "Now the operator dashboard. This interface is compiled into the orchestrator. "
        "It is not hosted on GitHub Pages. We will log in locally and operate one tenant.",
    ),
    (
        "dash_login",
        "Sign in as admin. Then provision a tenant. "
        "Name: Demo Acme. I D: demo-acme. Replicas: none. "
        "That is the certified single-node path.",
    ),
    (
        "dash_overview",
        "Overview shows live memory and command rate for this engine only. "
        "Backup from here is a point-in-time archive of this tenant — not a snapshot of the whole machine.",
    ),
    (
        "dash_keys",
        "Mint a writer key. Applications AUTH on port six three eight zero "
        "with tenant I D, key I D, and the secret. The secret is shown once. Copy it, then continue.",
    ),
    (
        "dash_console",
        "This console talks to the live engine. "
        "PING. Then SET session forty-two to onboarding. Then GET. "
        "You should see the same value come back. Same tenant. Same process.",
    ),
    (
        "dash_explorer",
        "Data Explorer lists the keys we just wrote. Open one to inspect it. "
        "You can also add a string key from here.",
    ),
    (
        "dash_vector",
        "VADD stores a vector. VSEARCH finds nearest neighbours. "
        "The vector playground is for semantic search. "
        "We will not download a large embedding model in this video.",
    ),
    (
        "runtime",
        "Hardware, storage, network, and hosting describe this process. "
        "Settings, security, and replication are control-plane. "
        "Replication is an async write-ahead log. It is not Raft.",
    ),
    (
        "close",
        "Self-host is free under B S L 1.1, including inside your own SaaS. "
        "You pay only if you sell managed DBX. Thank you for watching.",
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
    # Slow Cmaj7 / Am7 / F / G pad. Original, low, soothing.
    chords = [
        (130.81, 164.81, 196.00, 246.94),
        (110.00, 164.81, 196.00, 220.00),
        (87.31, 130.81, 174.61, 220.00),
        (98.00, 146.83, 196.00, 246.94),
    ]
    segs = []
    t = 0.0
    i = 0
    work = path.parent
    while t < seconds + 12:
        n1, n2, n3, n4 = chords[i % 4]
        seg = work / f"ch{i}.wav"
        d = 10.0
        run(
            [
                "ffmpeg",
                "-y",
                "-f",
                "lavfi",
                "-i",
                f"sine=frequency={n1}:duration={d}",
                "-f",
                "lavfi",
                "-i",
                f"sine=frequency={n2}:duration={d}",
                "-f",
                "lavfi",
                "-i",
                f"sine=frequency={n3}:duration={d}",
                "-f",
                "lavfi",
                "-i",
                f"sine=frequency={n4}:duration={d}",
                "-f",
                "lavfi",
                "-i",
                f"anoisesrc=color=brown:amplitude=0.04:duration={d}",
                "-filter_complex",
                "[0]volume=0.11[a];[1]volume=0.07[b];[2]volume=0.05[c];[3]volume=0.03[d];"
                "[4]volume=0.02,lowpass=f=160[n];"
                "[a][b][c][d][n]amix=inputs=5:duration=longest:normalize=0,"
                "lowpass=f=640,afade=t=in:d=1.2,afade=t=out:st=8.6:d=1.4",
                str(seg),
            ],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        segs.append(seg)
        t += 8.8
        i += 1
    lst = work / "chords.txt"
    lst.write_text("".join(f"file '{p}'\n" for p in segs))
    long = work / "music_long.wav"
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
            str(long),
        ],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    run(
        [
            "ffmpeg",
            "-y",
            "-i",
            str(long),
            "-t",
            str(seconds),
            "-af",
            "alimiter=limit=0.12,volume=0.55,afade=t=in:d=2,afade=t=out:st="
            + str(max(1.0, seconds - 4))
            + ":d=4",
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
    if not RAW.exists():
        raise SystemExit(f"missing raw capture {RAW}")
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

    intro = work / "intro.mp4"
    part2 = work / "part2.mp4"
    outro = work / "outro.mp4"
    card(intro, "PRODUCT WALKTHROUGH", "DBX", "Per-tenant memory for AI products", 8.5)
    card(
        part2,
        "PART TWO",
        "Operator dashboard",
        "Provision, keys, console, vectors",
        6.5,
    )
    card(outro, "BSL 1.1", "Self-host it.", "Free inside your own SaaS", 7.0)

    raw_dur = duration(RAW)
    keep = max(8.0, raw_dur - CURSOR_TRIM)
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

    dash_at = marks.get("dash_login", keep * 0.55)
    # Split body for the part-two card.
    site_len = max(4.0, dash_at - 0.15)
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
    picture = work / "picture.mp4"
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
    pic_dur = duration(picture)
    print("picture", pic_dur)

    # Map marks into the concatenated timeline.
    intro_d = duration(intro)
    part2_d = duration(part2)
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
        f"apad=whole_dur={pic_dur:.3f},atrim=0:{pic_dur:.3f},asetpts=PTS-STARTPTS,"
        f"loudnorm=I=-17:TP=-1.8:LRA=9[voice]"
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
            "[0]volume=1.08[v];[1]volume=0.14[m];"
            "[v][m]amix=inputs=2:duration=first:dropout_transition=0:normalize=0,"
            "alimiter=limit=0.96",
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
    print("wrote", OUT, f"{OUT.stat().st_size/1e6:.1f}MB", "dur", duration(OUT))
    shutil.rmtree(work, ignore_errors=True)


if __name__ == "__main__":
    os.environ.setdefault("PYTHONUNBUFFERED", "1")
    asyncio.run(main())
