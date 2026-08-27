/** Operator-console number formatting. Never print "0k" for values under 1000. */

export function formatMetric(n: number): string {
  if (!Number.isFinite(n)) return '—';
  const abs = Math.abs(n);
  if (abs < 1000) {
    return n % 1 === 0 ? String(n) : n.toFixed(2);
  }
  if (abs < 1_000_000) {
    return `${(n / 1000).toFixed(1)}k`;
  }
  return `${(n / 1_000_000).toFixed(2)}M`;
}

export function formatAxis(v: number): string {
  if (!Number.isFinite(v) || v === 0) return '0';
  if (Math.abs(v) < 1000) return String(Math.round(v));
  return `${(v / 1000).toFixed(0)}k`;
}

export function formatMemory(mb: number): { value: number; unit: string; label: string } {
  if (!Number.isFinite(mb) || mb <= 0) {
    return { value: 0, unit: 'MB', label: '0 MB' };
  }
  if (mb < 1024) {
    const value = mb >= 100 ? Math.round(mb) : Number(mb.toFixed(mb >= 10 ? 1 : 2));
    return { value, unit: 'MB', label: `${value} MB` };
  }
  const gb = mb / 1024;
  const value = Number(gb.toFixed(2));
  return { value, unit: 'GB', label: `${value} GB` };
}

export function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '0s';
  if (seconds < 60) return `${Math.floor(seconds)}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
  if (seconds < 86400) {
    const h = Math.floor(seconds / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    return m ? `${h}h ${m}m` : `${h}h`;
  }
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  return h ? `${d}d ${h}h` : `${d}d`;
}

export function formatClock(d = new Date()): string {
  const tz = typeof localStorage !== 'undefined' ? localStorage.getItem('dbx-timezone') : null;
  try {
    return d.toLocaleTimeString('en-GB', {
      hour12: false,
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      timeZone: tz || undefined,
    });
  } catch {
    return [
      String(d.getHours()).padStart(2, '0'),
      String(d.getMinutes()).padStart(2, '0'),
      String(d.getSeconds()).padStart(2, '0'),
    ].join(':');
  }
}
