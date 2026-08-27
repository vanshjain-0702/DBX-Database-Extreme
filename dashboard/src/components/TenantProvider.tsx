import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { useLocation } from 'react-router-dom';
import { fetchTenants, type Tenant } from '../api';

type TenantContextValue = {
  tenants: Tenant[];
  loading: boolean;
  error: string;
  refresh: () => Promise<void>;
};

const TenantContext = createContext<TenantContextValue | undefined>(undefined);

export function TenantProvider({ children }: { children: ReactNode }) {
  const location = useLocation();
  const [tenants, setTenants] = useState<Tenant[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const refresh = useCallback(async () => {
    if (!localStorage.getItem('dbx_token')) {
      setTenants([]);
      setLoading(false);
      return;
    }
    try {
      const list = await fetchTenants();
      setTenants(list);
      setError('');
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Failed to load tenants');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
    const id = window.setInterval(refresh, 5000);
    return () => window.clearInterval(id);
  }, [refresh, location.pathname]);

  const value = useMemo(
    () => ({ tenants, loading, error, refresh }),
    [tenants, loading, error, refresh]
  );

  return <TenantContext.Provider value={value}>{children}</TenantContext.Provider>;
}

export function useTenants() {
  const ctx = useContext(TenantContext);
  if (!ctx) throw new Error('useTenants must be used within TenantProvider');
  return ctx;
}

export function useTenant(id: string | undefined) {
  const { tenants, loading, error, refresh } = useTenants();
  return {
    tenant: tenants.find(t => t.id === id),
    tenants,
    loading,
    error,
    refresh,
  };
}
