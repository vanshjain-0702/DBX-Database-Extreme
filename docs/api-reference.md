# DBX API Reference

All data plane requests are proxied through the Orchestrator using the pattern:

```
POST /t/{tenantID}/query
Authorization: Bearer <jwt_token>
Content-Type: application/json

{ "command": ["COMMAND", "arg1", "arg2"] }
```

## Authentication

### POST `/api/login`
Get a JWT token.

**Request:**
```json
{ "username": "admin", "password": "yourpassword" }
```
**Response:**
```json
{ "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." }
```

---

## Tenant Management

### GET `/api/tenants`
List all provisioned tenants. Requires auth.

### POST `/api/provision`
Create a new tenant (database instance). Requires auth.

**Request:**
```json
{ "id": "my-app-prod", "name": "My Production App" }
```

---

## KV Commands

All standard Redis commands are supported via the query endpoint.

| Command | Example |
|---|---|
| `SET` | `["SET", "key", "value"]` |
| `GET` | `["GET", "key"]` |
| `DEL` | `["DEL", "key"]` |
| `KEYS` | `["KEYS", "*"]` |
| `HSET` | `["HSET", "hash", "field", "value"]` |
| `HGETALL` | `["HGETALL", "hash"]` |
| `LPUSH` | `["LPUSH", "list", "item"]` |
| `LRANGE` | `["LRANGE", "list", "0", "-1"]` |
| `SADD` | `["SADD", "set", "member"]` |
| `ZADD` | `["ZADD", "zset", "1.0", "member"]` |
| `EXPIRE` | `["EXPIRE", "key", "3600"]` |

---

## Vector Commands

| Command | Example |
|---|---|
| `VSET` | `["VSET", "doc:1", "0.1,0.2,0.9,..."]` |
| `VSEARCH` | `["VSEARCH", "0.1,0.2,0.8,...", "5"]` |
| `VGET` | `["VGET", "doc:1"]` |
| `VDEL` | `["VDEL", "doc:1"]` |

---

## Metrics

### GET `/t/{tenantID}/metrics`
Returns real-time performance metrics for the given tenant.

**Response:**
```json
{
  "total_commands": 150432,
  "avg_latency_ns": 3200,
  "memory_used_bytes": 52428800,
  "active_conns": 12
}
```
