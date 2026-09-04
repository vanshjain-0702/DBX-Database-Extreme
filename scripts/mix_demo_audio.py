#!/usr/bin/env python3
"""Add Indian-English narration + quiet music to website/assets/demo.mp4.

Uses Microsoft Edge neural voice en-IN-NeerjaNeural (clear Indian English).
Trims the Cursor brand tag at the end of the capture.
"""

from __future__ import annotations

import asyncio
import os
import shutil
import subprocess
import tempfile
from pathlib import Path

import edge_tts

SRC_VIDEO = Path("/opt/cursor/artifacts/dbx-product-walkthrough.mp4")
FALLBACK_SRC = Path("/workspace/website/assets/demo.mp4")
OUT_VIDEO = Path("/workspace/website/assets/demo.mp4")
# Cursor brand tag is ~2s at the end of a 222.28s capture; keep the site close.
CONTENT_END = 219.6
VOICE = "en-IN-NeerjaNeural"
RATE = "-8%"
PITCH = "-2Hz"

# Short sentences, Indian English, paced to the screen recording.
CUES: list[tuple[float, str]] = [
    (
        0.6,
        "Hello. Welcome to D B X — a per-tenant memory engine for A I products. "
        "First the public website, then the operator dashboard.",
    ),
    (
        8.4,
        "Home page. One isolated engine per customer. State and vectors share one process. "
        "Isolation is a directory, a write-ahead log, and an H N S W index — not a key prefix.",
    ),
    (
        20.0,
        "This isolation bench is a browser sketch, not a live node. "
        "Tenants are acme, harbor, and lumen. You can type AUTH, SET, GET, and VADD.",
    ),
    (
        31.0,
        "AUTH to harbor, SET a session, then AUTH to acme and GET. "
        "Acme cannot see harbor's data. Each cabinet is a different engine.",
    ),
    (
        42.0,
        "Shared clusters make tenancy your job. One missing prefix can leak another customer's data. "
        "In D B X, the tenant is a first-class object.",
    ),
    (
        52.0,
        "Control K jumps across pages. Five claims: first-class tenant, state plus vectors, "
        "cost on active tenants, one binary, and Isolation Kernel on Linux strict.",
    ),
    (
        65.0,
        "Run it locally. Dashboard on port eight thousand. Public RESP on port six three eight zero. "
        "Docker, source, or Compose. First command is AUTH with tenant I D, key I D, and secret.",
    ),
    (
        79.0,
        "Certified on one node: about one lakh eighty-six thousand SETs per second, "
        "two lakh eighty-five thousand GETs, recall at ten of zero point nine two. Measure on your hardware.",
    ),
    (
        93.5,
        "Docs cover provision, backup, restore, hibernate and wake. "
        "Durable commands: SET, GET, VADD, VSEARCH. Raft and cluster fail closed in version one.",
    ),
    (
        106.0,
        "Isolation Kernel means own process, Landlock, encrypted log. "
        "make run-dev is in-process — not that claim. Mail is not live yet; use GitHub issues.",
    ),
    (
        119.5,
        "Now the operator dashboard. This U I ships inside the orchestrator. It is not on GitHub Pages.",
    ),
    (
        128.5,
        "Sign in as admin. Provision tenant Demo Acme, I D demo-acme, no replicas. That is the certified path.",
    ),
    (
        138.5,
        "Overview shows live memory and ops. Backup from here is a point-in-time snapshot of this tenant.",
    ),
    (
        146.5,
        "Mint a writer key for AUTH on port six three eight zero. The secret is shown once. Copy it.",
    ),
    (
        157.0,
        "Console talks to the live engine. PING. SET session forty-two. GET returns onboarding.",
    ),
    (
        168.8,
        "VADD stores a vector. VSEARCH finds neighbours. Explorer lists keys. "
        "Vector Playground is semantic search; we skip the big model download.",
    ),
    (
        183.5,
        "Hardware, storage, network, hosting. Then settings, security, replication. "
        "Replication is async write-ahead log, not Raft.",
    ),
    (
        201.0,
        "Self-host is free under B S L 1.1, including inside your own SaaS. "
        "Paid only if you sell managed D B X. Thank you for watching.",
    ),
]


def run(cmd: list[str]) -> None:
    subprocess.check_call(cmd)


def ffprobe_duration(path: Path) -> float:
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


async def synth_cue(text: str, dest: Path) -> None:
    communicate = edge_tts.Communicate(text, VOICE, rate=RATE, pitch=PITCH)
    tmp = dest.with_suffix(".mp3")
    await communicate.save(str(tmp))
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
            "-c:a",
            "pcm_s16le",
            str(dest),
        ]
    )
    tmp.unlink(missing_ok=True)


def make_music(path: Path, duration: float) -> None:
    # Quiet low pad: stacked sines + brown noise. Original, not a commercial track.
    run(
        [
            "ffmpeg",
            "-y",
            "-f",
            "lavfi",
            "-i",
            f"sine=frequency=87.31:sample_rate=44100:duration={duration}",
            "-f",
            "lavfi",
            "-i",
            f"sine=frequency=130.81:sample_rate=44100:duration={duration}",
            "-f",
            "lavfi",
            "-i",
            f"sine=frequency=164.81:sample_rate=44100:duration={duration}",
            "-f",
            "lavfi",
            "-i",
            f"sine=frequency=196.00:sample_rate=44100:duration={duration}",
            "-f",
            "lavfi",
            "-i",
            f"anoisesrc=color=brown:amplitude=0.12:sample_rate=44100:duration={duration}",
            "-filter_complex",
            "[0]volume=0.10,lowpass=f=220[a];"
            "[1]volume=0.07,lowpass=f=320[b];"
            "[2]volume=0.045,lowpass=f=420[c];"
            "[3]volume=0.03,lowpass=f=500[d];"
            "[4]volume=0.035,lowpass=f=180,highpass=f=35[n];"
            "[a][b][c][d][n]amix=inputs=5:duration=longest:normalize=0,"
            "tremolo=f=0.12:d=0.18,treble=g=-10,alimiter=limit=0.15,volume=0.45",
            "-c:a",
            "pcm_s16le",
            str(path),
        ]
    )


async def main() -> None:
    src = SRC_VIDEO if SRC_VIDEO.exists() else FALLBACK_SRC
    if not src.exists():
        raise SystemExit("missing source video")

    work = Path(tempfile.mkdtemp(prefix="dbx-mix-"))
    try:
        clips: list[tuple[float, Path, float]] = []
        for i, (start, text) in enumerate(CUES):
            wav = work / f"cue_{i:02d}.wav"
            print(f"tts {i+1}/{len(CUES)} @ {start:.1f}s")
            await synth_cue(text, wav)
            dur = ffprobe_duration(wav)
            window = (
                (CUES[i + 1][0] - start - 0.45)
                if i + 1 < len(CUES)
                else (CONTENT_END - start - 0.5)
            )
            if window > 0.4 and dur > window:
                sped = work / f"cue_{i:02d}_spd.wav"
                tempo = min(1.36, max(1.01, dur / window))
                run(
                    [
                        "ffmpeg",
                        "-y",
                        "-i",
                        str(wav),
                        "-filter:a",
                        f"atempo={tempo:.4f}",
                        str(sped),
                    ]
                )
                wav = sped
                dur = ffprobe_duration(wav)
            clips.append((start, wav, dur))
            print(f"  {dur:.2f}s (window {window:.2f}s)")

        n = len(clips)
        inputs: list[str] = []
        filters = []
        for i, (start, wav, _dur) in enumerate(clips):
            inputs += ["-i", str(wav)]
            delay_ms = int(round(start * 1000))
            filters.append(f"[{i}]adelay={delay_ms}|{delay_ms}[v{i}]")
        mix_in = "".join(f"[v{i}]" for i in range(n))
        filters.append(
            f"{mix_in}amix=inputs={n}:duration=longest:dropout_transition=0:normalize=0,"
            f"atrim=0:{CONTENT_END},asetpts=PTS-STARTPTS,loudnorm=I=-16:TP=-1.5:LRA=11[voice]"
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
                "-c:a",
                "pcm_s16le",
                str(voice),
            ]
        )

        music = work / "music.wav"
        make_music(music, CONTENT_END)

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
                "[0]volume=1.05,apad=whole_dur=" + str(CONTENT_END) + "[v];"
                "[1]volume=0.16[m];"
                "[v][m]amix=inputs=2:duration=first:dropout_transition=0:normalize=0,"
                "alimiter=limit=0.95,atrim=0:"
                + str(CONTENT_END)
                + ",asetpts=PTS-STARTPTS",
                "-c:a",
                "pcm_s16le",
                str(mixed),
            ]
        )

        trimmed = work / "trimmed.mp4"
        run(
            [
                "ffmpeg",
                "-y",
                "-i",
                str(src),
                "-t",
                str(CONTENT_END),
                "-an",
                "-c:v",
                "libx264",
                "-preset",
                "fast",
                "-crf",
                "20",
                "-pix_fmt",
                "yuv420p",
                str(trimmed),
            ]
        )

        staged = work / "demo_out.mp4"
        run(
            [
                "ffmpeg",
                "-y",
                "-i",
                str(trimmed),
                "-i",
                str(mixed),
                "-c:v",
                "copy",
                "-c:a",
                "aac",
                "-b:a",
                "160k",
                "-ar",
                "44100",
                "-t",
                str(CONTENT_END),
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
        shutil.copyfile(staged, OUT_VIDEO)
        print("wrote", OUT_VIDEO, f"{OUT_VIDEO.stat().st_size / 1e6:.1f} MB")
        print("duration", ffprobe_duration(OUT_VIDEO))
    finally:
        shutil.rmtree(work, ignore_errors=True)


if __name__ == "__main__":
    os.environ.setdefault("PYTHONUNBUFFERED", "1")
    asyncio.run(main())
