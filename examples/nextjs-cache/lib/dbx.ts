// lib/dbx.ts — Singleton DBX (RESP) client for Next.js server components.
//
// DBX speaks the RESP protocol, so the standard `redis` npm client connects
// without a custom driver. This is an adoption on-ramp, not a claim that DBX
// substitutes for a tuned Redis cluster.
//
// The first command on :6380 must be AUTH tenantID:keyID secret.

import { createClient, type RedisClientType } from 'redis';

let client: RedisClientType | undefined;

export async function dbx(): Promise<RedisClientType> {
  if (!client) {
    client = createClient({
      url: process.env.DBX_URL || 'redis://127.0.0.1:6380',
    });
    await client.connect();
    await client.sendCommand([
      'AUTH',
      `${process.env.DBX_TENANT}:${process.env.DBX_KEY_ID}`,
      process.env.DBX_SECRET as string,
    ]);
  }
  return client;
}
