# DBX incubation pitch

Live deck (after Pages deploy):
[https://vanshjain-0702.github.io/DBX-Database-Extreme/pitch.html](https://vanshjain-0702.github.io/DBX-Database-Extreme/pitch.html)

Local: `make site` then open http://127.0.0.1:8765/pitch.html

## File this in the meeting

| Format | How |
|---|---|
| Present live | Full screen the HTML deck. Arrow keys, Space, or on-screen Prev/Next. `N` toggles speaker notes. `Home` / `End` jump. |
| PDF | Open `pitch.html?print=1` then Chrome → Print → Save as PDF → **Landscape**, margins **None**, background graphics **On**. Or click **Print / PDF** in the live deck. |
| Link in the application form | `https://vanshjain-0702.github.io/DBX-Database-Extreme/pitch.html` |
| PPTX (file for the committee) | `make pitch-pptx` → `website/assets/dbx-incubation-pitch.pptx`. Commit of the generated file is in `website/assets/`. |

Do not paste a Word outline. The HTML/PDF is the artifact.

## What this deck is allowed to say

Source of truth remains [`docs/positioning.md`](../docs/positioning.md) and [`docs/isolation.md`](../docs/isolation.md).

- Say: per-tenant memory engine; tenant is the unit; existing RESP clients work; cost per active tenant; Isolation Kernel on Linux `strict`.
- Do not say: drop-in Redis replacement; strongest sandbox; SOC 2; invented ARR or customer logos; encrypted `.vec` rows; sub-millisecond ANN (certified p50 is 2.304 ms).

Market figures on slide 13 are third-party order-of-magnitude citations (MarketsandMarkets, Mordor Intelligence, HTF). They are not a DBX forecast.

The ask is an **18-month mix** (45 / 20 / 20 / 10 / 5). Scale the currency envelope to the incubator; do not invent a valuation on the slide.

## Suggested 10-minute path

1. Cover + thesis (slides 1–2)  
2. Problem + solution (4–5)  
3. USPs + Isolation Kernel (6–7)  
4. Traction numbers (11–12)  
5. Model + ask (14, 16)  
6. Close (19)

Skip appendix (20) unless asked “what will you never build.”

## Contact until MX exists

GitHub issues deliver today. `hello@dbxdb.io` is the license contact and is not receiving mail until `dbxdb.io` has DNS/MX.
