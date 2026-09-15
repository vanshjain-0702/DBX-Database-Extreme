# DBX on DEV.to

This folder is the paste kit for DEV Community posts. Do not invent extra tags, titles, or URLs. Each publish guide is the source of truth for editor metadata; `docs/isolation.md` and `docs/positioning.md` are the source of truth for claims.

| Post | Article (paste body) | Editor walkthrough |
|---|---|---|
| Product / tutorial (series `DBX`) | [the-tenant-is-the-database.md](the-tenant-is-the-database.md) | [DEVTO_PUBLISH_TENANT.md](DEVTO_PUBLISH_TENANT.md) |
| Isolation Kernel (series `DBX Isolation Kernel`) | [building-secure-multi-tenant-ai-memory.md](building-secure-multi-tenant-ai-memory.md) | [DEVTO_PUBLISH.md](DEVTO_PUBLISH.md) |

Publish the product post first if you have only one slot: it is the on-ramp. Publish the Isolation Kernel post when you want the Landlock / DEK / `SO_PEERCRED` walkthrough. After the kernel post is live, swap the GitHub blob link in the product post for that DEV.to URL (see Step 4 in the tenant kit).
