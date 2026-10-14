import { useEffect, useState, type RefObject } from 'react';
import { Menu, LogOut, ChevronRight } from 'lucide-react';
import { Link, useNavigate, useLocation } from 'react-router-dom';
import { api, message, type Health } from '../lib/api';
import { useSession } from '../lib/session';
export default function Header({ openNavigation, navigationOpen, navigationButtonRef }: {
    openNavigation: () => void; navigationOpen: boolean; navigationButtonRef: RefObject<HTMLButtonElement | null>;
}) {
    const {user, reload} = useSession();
    const [health, setHealth] = useState<Health | null>(null);
    const [checked, setChecked] = useState(false);
    const [error, setError] = useState('');
    const [busy, setBusy] = useState(false);
    const navigate = useNavigate();
    const location = useLocation();
    const section = location.pathname.split('/')[1];
    const labels: Record<string, string> = {matters: 'Matters', checklists: 'Checklists', regulations: 'Regulations', exports: 'Exports', firm: 'Firm & connection'};
    useEffect(() => {
        let active = true;
        const check = () => api<Health>('/api/readiness').then(value => { if (active) { setHealth(value); setChecked(true); } }).catch(() => { if (active) { setHealth(null); setChecked(true); } });
        void check(); const timer = setInterval(check, 30000);
        return () => { active = false; clearInterval(timer); };
    }, []);
    async function logout() {
        setBusy(true); setError('');
        try { await api('/api/logout', {method: 'POST'}); localStorage.removeItem('jurix_token'); await reload(); navigate('/login'); }
        catch (error) { setError(message(error)); } finally { setBusy(false); }
    }
    return <header className="app-header">
        <button className="icon-button menu-button" ref={navigationButtonRef} onClick={openNavigation} aria-expanded={navigationOpen} aria-controls="primary-navigation" aria-label="Toggle navigation"><Menu size={22}/></button>
        <div className="header-breadcrumb"><span>{user?.firm_name || 'Workspace'}</span><ChevronRight size={13}/><strong>{labels[section] || 'Workspace'}</strong></div>
        <Link to="/firm" className="connection"><span className={`connection-dot ${health?.dbx?.ok ? 'online' : ''}`}/>{!checked ? 'Checking storage' : !health ? 'Connection unverified' : health.dbx?.ok ? 'DBX connected' : 'Storage unavailable'}</Link>
        <Link to="/firm" className="header-user"><span>{user?.name || 'Your account'}<small>{user?.role || 'Account'}</small></span><b>{user?.initials || 'JX'}</b></Link>
        <button className="icon-button" disabled={busy} onClick={logout} aria-label="Sign out" title="Sign out"><LogOut size={18}/></button>
        {error && <span className="header-error" role="alert">{error}</span>}
    </header>;
}
