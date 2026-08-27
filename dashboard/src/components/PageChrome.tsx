import { useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { fetchWithAuth, statusLabel, type Tenant } from '../api';
import { useTenant } from './TenantProvider';

export function StatusBadge({ tenant }: { tenant: Tenant }) {
  return (
    <span className={`status-badge ${tenant.status}`}>
      <span className="status-dot" />
      {statusLabel(tenant.status)}
    </span>
  );
}

export function StatusStrip({ clusterId }: { clusterId: string }) {
  const { tenant } = useTenant(clusterId);
  const [keyCount, setKeyCount] = useState<number | null>(null);

  useEffect(() => {
    if (!tenant || tenant.status === 'down') {
      setKeyCount(null);
      return;
    }
    let cancelled = false;
    (async () => {
      try {
        const res = await fetchWithAuth(`/t/${clusterId}/api/keyspace`);
        if (!res.ok) return;
        const data = await res.json();
        if (!data || typeof data !== 'object') return;
        const n = Object.values(data as Record<string, unknown>).reduce<number>(
          (sum, v) => sum + (typeof v === 'number' ? v : 0),
          0
        );
        if (!cancelled) setKeyCount(n);
      } catch {
        if (!cancelled) setKeyCount(null);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [clusterId, tenant?.status]);

  if (!tenant) return null;

  return (
    <>
      <div className="status-strip" role="status">
        <span className="strip-item">
          <StatusBadge tenant={tenant} />
        </span>
        <span className="strip-item">
          HTTP <span className="strip-mono">:{tenant.http_port || '—'}</span>
        </span>
        <span className="strip-item">
          RESP <span className="strip-mono">:{tenant.resp_port || '—'}</span>
        </span>
        <span className="strip-item">
          Keys <span className="strip-mono">{keyCount === null ? '—' : keyCount}</span>
        </span>
      </div>
      {tenant.status === 'down' && (
        <div className="banner-down">Engine unreachable. This tenant is down — metrics and queries will fail until it is running.</div>
      )}
    </>
  );
}

export default function PageChrome({
  title,
  purpose,
  clusterId,
  extra,
}: {
  title: string;
  purpose: string;
  clusterId: string;
  extra?: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-3">
      <div className="page-header">
        <div>
          <h1 className="page-title">{title}</h1>
          <p className="page-purpose">{purpose}</p>
        </div>
        {extra}
      </div>
      <StatusStrip clusterId={clusterId} />
    </div>
  );
}
