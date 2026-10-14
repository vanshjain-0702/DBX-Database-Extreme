import { useCallback, useEffect, useRef, useState, type Dispatch, type SetStateAction } from 'react';
import { useParams, Link, Outlet, NavLink, useOutletContext } from 'react-router-dom';
import { ArrowLeft, LayoutDashboard, CheckSquare, FileText, Search, Receipt } from 'lucide-react';
import { api, message, type Matter } from '../lib/api';
import { Loading, ErrorNotice } from '../components/Feedback';
import ProcessingConsent from '../components/ProcessingConsent';
export interface MatterContext {
    matter: Matter;
    refreshMatter: () => Promise<Matter>;
    setMatter: Dispatch<SetStateAction<Matter | null>>;
}
export const useMatter = () => useOutletContext<MatterContext>();
export default function MatterLayout() {
    const { id } = useParams();
    const [matter, setMatterState] = useState<Matter | null>(null);
    const currentId = useRef(id);
    currentId.current = id;
    const setMatter: Dispatch<SetStateAction<Matter | null>> = useCallback(next => {
        setMatterState(previous => {
            const value = typeof next === 'function' ? next(previous) : next;
            return value && value.id !== currentId.current ? previous : value;
        });
    }, []);
    const [error, setError] = useState('');
    const [loading, setLoading] = useState(true);
    const refreshMatter = useCallback(async () => { const data = await api<Matter>(`/api/matters/${id}`); setMatter(data); return data; }, [id]);
    useEffect(() => { let active = true; setLoading(true); setError(''); api<Matter>(`/api/matters/${id}`).then(data => { if (active)
        setMatter(data); }).catch(error => { if (active)
        setError(message(error)); }).finally(() => { if (active)
        setLoading(false); }); return () => { active = false; }; }, [id]);
    if (loading)
        return <Loading />;
    if (error || !matter)
        return <ErrorNotice error={error || 'Matter not found.'} retry={() => { setLoading(true); refreshMatter().then(() => setError('')).catch(error => setError(message(error))).finally(() => setLoading(false)); }}/>;
    return <div className="page-enter"><Link to="/matters" className="back-link"><ArrowLeft size={15}/> All matters</Link>
    <div className="matter-heading"><div><p className="eyebrow">{matter.name}</p><h1>{matter.matter}</h1><p className="muted">Reviewer: {matter.reviewer_name || 'Unassigned'}</p></div><span className="matter-id">MATTER / {matter.id.slice(0, 8).toUpperCase()}</span></div>
    <nav className="matter-tabs" aria-label="Matter navigation">{[{ name: 'Overview', path: 'dashboard', icon: LayoutDashboard }, { name: 'Findings', path: 'findings', icon: CheckSquare }, { name: 'Documents', path: 'documents', icon: FileText }, { name: 'Document search', path: 'counsel', icon: Search }, { name: 'Pre-bill review', path: 'billing', icon: Receipt }].map(({ name, path, icon: Icon }) => <NavLink key={path} to={path} className={({ isActive }) => isActive ? 'active' : ''}><Icon size={16}/>{name}</NavLink>)}</nav>
    <ProcessingConsent matter={matter} refreshMatter={refreshMatter}/>
    <Outlet context={{ matter, refreshMatter, setMatter } satisfies MatterContext}/>
  </div>;
}
