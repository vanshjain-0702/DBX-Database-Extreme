# Public site

The live site is GitHub Pages:

**https://vanshjain-0702.github.io/DBX-Database-Extreme/**

`dbxdb.io` does not resolve today (no DNS / the name is not registered), so
a click on that hostname cannot open a page. Contact mail
`hello@dbxdb.io` is the intended address once the domain exists.

## Enable Pages

Repo **Settings → Pages → Source = GitHub Actions**. The workflow is
[`.github/workflows/pages.yml`](../.github/workflows/pages.yml). It publishes
this directory. If the repository is private, anonymous visitors get 404
until Pages is public or they are signed in with access.

## Attach dbxdb.io later

1. Register `dbxdb.io` at a registrar.
2. Apex A records (GitHub Pages):

   ```
   185.199.108.153
   185.199.109.153
   185.199.110.153
   185.199.111.153
   ```

   Optional: `www` CNAME to `vanshjain-0702.github.io`.
3. Put a file `website/CNAME` containing `dbxdb.io` and push `main`.
4. In **Settings → Pages**, set the custom domain to `dbxdb.io` and wait for
   the TLS certificate.
