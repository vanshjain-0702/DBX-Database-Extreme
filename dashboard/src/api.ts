export const fetchWithAuth = async (url: string, options: RequestInit = {}) => {
  const token = localStorage.getItem('dbx_token');

  const headers = new Headers(options.headers || {});
  if (token) {
    headers.set('Authorization', `Bearer ${token}`);
  }

  const res = await fetch(url, { ...options, headers });
  if (res.status === 401) {
    localStorage.removeItem('dbx_token');
    window.location.href = '/login';
  }
  return res;
};

export type TenantStatus = 'running' | 'starting' | 'down';

export interface Tenant {
  id: string;
  name: string;
  http_port: number;
  resp_port: number;
  status: TenantStatus;
  healthy: boolean;
  engine: string;
  role?: string;
  replica_of?: string;
  replication_port?: number;
  replicas?: string[];
}

function asStatus(value: unknown): TenantStatus | undefined {
  if (value === 'running' || value === 'starting' || value === 'down') return value;
  return undefined;
}

export function normalizeTenant(raw: unknown): Tenant | null {
  if (!raw || typeof raw !== 'object') return null;
  const row = raw as Record<string, unknown>;
  const id = typeof row.id === 'string' ? row.id : '';
  if (!id) return null;
  const status = asStatus(row.status);
  return {
    id,
    name: typeof row.name === 'string' && row.name ? row.name : id,
    http_port: Number(row.http_port) || 0,
    resp_port: Number(row.resp_port) || 0,
    status: status ?? 'down',
    healthy: Boolean(row.healthy),
    engine: typeof row.engine === 'string' && row.engine ? row.engine : 'DBX',
    role: typeof row.role === 'string' ? row.role : '',
    replica_of: typeof row.replica_of === 'string' ? row.replica_of : '',
    replication_port: Number(row.replication_port) || 0,
    replicas: Array.isArray(row.replicas) ? row.replicas.filter((id): id is string => typeof id === 'string') : [],
  };
}

async function probeTenantStatus(id: string): Promise<{ status: TenantStatus; healthy: boolean }> {
  // /health is unauthenticated and can 200 on a foreign process bound to the
  // same port. /metrics requires the orchestrator-injected internal token.
  try {
    const metrics = await fetchWithAuth(`/t/${id}/metrics`);
    if (metrics.ok) return { status: 'running', healthy: true };
  } catch {
    /* down */
  }
  return { status: 'down', healthy: false };
}

export async function fetchTenants(): Promise<Tenant[]> {
  const res = await fetchWithAuth('/api/tenants');
  if (!res.ok) throw new Error(`Failed to list tenants (${res.status})`);
  const data = await res.json();
  const rows = Array.isArray(data) ? data : [];
  const tenants: Tenant[] = [];
  for (const raw of rows) {
    const tenant = normalizeTenant(raw);
    if (!tenant) continue;
    const listed = raw && typeof raw === 'object' ? (raw as Record<string, unknown>) : {};
    if (asStatus(listed.status) === undefined) {
      const probed = await probeTenantStatus(tenant.id);
      tenant.status = probed.status;
      tenant.healthy = probed.healthy;
    }
    tenants.push(tenant);
  }
  return tenants;
}

export function statusLabel(status: TenantStatus): string {
  if (status === 'running') return 'Running';
  if (status === 'starting') return 'Starting';
  return 'Down';
}

export function openCommandPalette() {
  window.dispatchEvent(new CustomEvent('dbx:open-palette'));
}
