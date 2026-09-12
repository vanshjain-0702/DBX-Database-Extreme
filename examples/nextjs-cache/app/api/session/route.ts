// app/api/session/route.ts — Next.js route handler using DBX for session cache.
//
// One tenant stores both session state (KV with TTL) and that customer's
// vector memory. There is no second system to keep in sync.

import { dbx } from '@/lib/dbx';

export async function GET() {
  const store = await dbx();
  const key = 'session:user:123';
  const cached = await store.get(key);
  if (cached) {
    return Response.json(JSON.parse(cached), {
      headers: { 'X-Store': 'HIT' },
    });
  }

  const session = { userId: 123, step: 'onboarding' };
  // SETEX is the node-redis setEx path; SET … EX is equivalent.
  await store.setEx(key, 3600, JSON.stringify(session));
  return Response.json(session, { headers: { 'X-Store': 'MISS' } });
}
