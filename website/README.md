# Public site

**Open it now:** `website/index.html` in a browser, or from the repo root
`make site`. That does not use DNS or GitHub Pages.

`https://dbxdb.io` does not resolve (the name is not registered).
`https://vanshjain-0702.github.io/DBX-Database-Extreme/` returns **404**
while this GitHub repository is **private** (GitHub Pages on Free is for
public repos). Making the repo public, or GitHub Pro, is required for that
URL to load.

## Publish on GitHub Pages

1. **Settings → Actions → General → Workflow permissions** → Read and write.
2. Push `main` (or run **Actions → Deploy site → Run workflow**). That updates
   the `gh-pages` branch from this folder.
3. **Settings → Pages → Deploy from a branch** → `gh-pages` / `/ (root)` → Save.
4. For people who are not logged in: **Settings → General → Change repository
   visibility → Public** (or use GitHub Pro with Pages enabled).

## Attach dbxdb.io later

1. Register `dbxdb.io`.
2. Apex A records:

   ```
   185.199.108.153
   185.199.109.153
   185.199.110.153
   185.199.111.153
   ```

3. Add `website/CNAME` containing `dbxdb.io`, push, and set the custom domain
   under **Settings → Pages**.
