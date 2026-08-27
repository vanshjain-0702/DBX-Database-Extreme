# Next.js Session Caching with DBX

Use DBX as a session and API response cache in your Next.js app. DBX speaks RESP, so the
standard `redis` npm client works without a custom driver.

The reason to reach for DBX here rather than a plain cache is that the same tenant also holds
your vector memory: session state and semantic recall for a customer live in one engine and one
data directory, so there is no second system to keep in sync.

## Setup

```bash
npm install redis
```

## Usage in Next.js API Routes

```typescript
// lib/dbx.ts
import { createClient } from 'redis';

const dbx = createClient({
  url: process.env.DBX_URL || 'redis://localhost:6380',
});

export default dbx;

// app/api/user/route.ts
import dbx from '@/lib/dbx';

export async function GET(request: Request) {
  const userId = "user:123";

  // Try cache first
  const cached = await dbx.get(userId);
  if (cached) {
    return Response.json(JSON.parse(cached), { headers: { 'X-Cache': 'HIT' } });
  }

  // Fetch from DB
  const user = await fetchUserFromDatabase(userId);

  // Store in DBX with 1-hour TTL
  await dbx.setEx(userId, 3600, JSON.stringify(user));

  return Response.json(user, { headers: { 'X-Cache': 'MISS' } });
}
```

## Environment Variables

```bash
# .env.local
DBX_URL=redis://localhost:6380
```
