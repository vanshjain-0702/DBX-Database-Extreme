# Next.js session state with DBX

Use one tenant as the working-state store for a Next.js app. DBX speaks RESP, so
the `redis` npm client connects without a custom driver — that is an on-ramp, not
a claim that DBX substitutes for a tuned cache cluster.

The reason to put session JSON here is the same tenant also holds that customer's
vector memory. Session state and semantic recall live in one engine and one data
directory. There is no second system to keep in sync.

The first command on `:6380` must be `AUTH tenantID:keyID secret`. `SETEX` and
`SET key value EX seconds` are both in the durable v1 string surface.

## Setup

```bash
npm install redis
```

Provision a tenant and mint a writer key first (`examples/quickstart.py` or the
dashboard). Then:

```bash
# .env.local
DBX_URL=redis://127.0.0.1:6380
DBX_TENANT=acme-corp
DBX_KEY_ID=key-id
DBX_SECRET=one-time-key-secret
```

## Usage in a Next.js route

```typescript
// lib/dbx.ts
import { createClient, type RedisClientType } from 'redis';

let client: RedisClientType | undefined;

export async function dbx(): Promise<RedisClientType> {
  if (!client) {
    client = createClient({ url: process.env.DBX_URL || 'redis://127.0.0.1:6380' });
    await client.connect();
    await client.sendCommand([
      'AUTH',
      `${process.env.DBX_TENANT}:${process.env.DBX_KEY_ID}`,
      process.env.DBX_SECRET as string,
    ]);
  }
  return client;
}

// app/api/session/route.ts
import { dbx } from '@/lib/dbx';

export async function GET() {
  const store = await dbx();
  const key = 'session:user:123';
  const cached = await store.get(key);
  if (cached) {
    return Response.json(JSON.parse(cached), { headers: { 'X-Store': 'HIT' } });
  }

  const session = { userId: 123, step: 'onboarding' };
  // SETEX is the node-redis setEx path; SET … EX is equivalent.
  await store.setEx(key, 3600, JSON.stringify(session));
  return Response.json(session, { headers: { 'X-Store': 'MISS' } });
}
```

Point this at one tenant. Do not share a connection across customers — mint a key
per tenant and AUTH that identity.
