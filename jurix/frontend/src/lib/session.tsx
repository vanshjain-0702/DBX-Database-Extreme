import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react';
import { api, message, type User, type Capabilities } from './api';

const SessionContext = createContext<{ user: User | null; loading: boolean; error: string; capabilities: Capabilities | null; reload: () => Promise<void> }>({ user: null, loading: true, error: '', capabilities: null, reload: async () => { } });
export function SessionProvider({ children }: { children: ReactNode }) {
    const [user, setUser] = useState<User | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');
    const [capabilities, setCapabilities] = useState<Capabilities | null>(null);
    const revision = useRef(0);
    const reload = useCallback(async () => {
        const requestId = ++revision.current;
        setLoading(true); setError('');
        try {
            const session = await api<{ authenticated: boolean; user?: User }>('/api/session', {}, false);
            const nextUser = session.authenticated ? session.user || await api<User>('/api/me') : null;
            let nextCapabilities: Capabilities | null = null;
            if (nextUser) {
                try { nextCapabilities = await api<Capabilities>('/api/capabilities'); }
                catch { /* Preserve an explicit, unverified provider state. */ }
            }
            if (requestId !== revision.current) return;
            setUser(nextUser); setCapabilities(nextCapabilities);
        } catch (error) {
            if (requestId !== revision.current) return;
            setUser(null); setCapabilities(null); setError(message(error));
        } finally { if (requestId === revision.current) setLoading(false); }
    }, []);
    useEffect(() => {
        void reload();
        const expired = () => { revision.current++; setUser(null); setCapabilities(null); setError(''); setLoading(false); };
        window.addEventListener('jurix-session-expired', expired);
        return () => { revision.current++; window.removeEventListener('jurix-session-expired', expired); };
    }, [reload]);
    return <SessionContext.Provider value={{ user, loading, error, capabilities, reload }}>{children}</SessionContext.Provider>;
}
export const useSession = () => useContext(SessionContext);
