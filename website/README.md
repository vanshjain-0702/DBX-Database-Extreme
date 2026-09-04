# Public site

**Live:** [https://vanshjain-0702.github.io/DBX-Database-Extreme/](https://vanshjain-0702.github.io/DBX-Database-Extreme/)

**Show demo video:** [Walkthrough](https://vanshjain-0702.github.io/DBX-Database-Extreme/demo.html) (`demo.html` + [`assets/demo.mp4`](assets/demo.mp4)). Also linked from the root [README](../README.md).

That is the URL to open from GitHub. Source for it is this folder; GitHub Actions
deploys it from `main` (workflow [Deploy site](../.github/workflows/pages.yml)).

Local preview from the repo root: `make site` (http://127.0.0.1:8765/).

`dbxdb.io` is the intended custom domain once it is registered. Until then the
canonical host is github.io. Do not add `website/CNAME` until DNS for that name
points at GitHub Pages — a CNAME without DNS takes the github.io site offline.

## Publish

1. **Settings → Pages → Source** → GitHub Actions (already set on this repo).
2. Push `main`, or **Actions → Deploy site → Run workflow**.
3. Optional: **Settings → General → About → Website** → paste the github.io URL
   so the globe link appears on the repository page.

The repo must stay **public** for GitHub Pages on a Free plan.
