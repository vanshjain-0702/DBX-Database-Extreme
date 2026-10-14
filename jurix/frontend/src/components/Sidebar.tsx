import { NavLink } from 'react-router-dom';
import { Scale, CheckSquare, Download, Settings, ArrowUpRight, X, Landmark } from 'lucide-react';
import type { RefObject } from 'react';
export default function Sidebar({ open, close, hidden, drawerRef, mobile }: {
    open: boolean;
    close: () => void;
    hidden: boolean;
    drawerRef: RefObject<HTMLElement | null>;
    mobile: boolean;
}) {
    return <aside className={`sidebar ${open ? 'is-open' : ''}`} id="primary-navigation" ref={drawerRef} inert={hidden} aria-hidden={hidden || undefined} role={mobile && open ? 'dialog' : undefined} aria-modal={mobile && open ? true : undefined} aria-label={mobile && open ? 'Workspace navigation' : undefined}>
    {mobile && open && <button className="icon-button drawer-close" aria-label="Close navigation" onClick={close}><X size={19}/></button>}
    <NavLink to="/matters" className="brand" onClick={close}><Scale size={30}/><span>JURIX<small>CONTRACT REVIEW</small></span></NavLink>
    <div className="nav-caption">Your workspace</div>
    <nav aria-label="Main navigation">
      {[{ name: 'Matters', to: '/matters', icon: Scale }, { name: 'Checklists', to: '/checklists', icon: CheckSquare }, { name: 'Regulations', to: '/regulations', icon: Landmark }, { name: 'Exports', to: '/exports', icon: Download }].map(({ name, to, icon: Icon }) => <NavLink key={to} to={to} onClick={close} className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}><Icon size={18}/>{name}<ArrowUpRight className="nav-arrow" size={14}/></NavLink>)}
    </nav>
    <div className="sidebar-bottom"><NavLink to="/firm" onClick={close} className={({ isActive }) => `nav-link ${isActive ? 'active' : ''}`}><Settings size={18}/>Firm & connection</NavLink>
      <p>A clear record.<br /><span>A considered decision.</span></p>
    </div>
  </aside>;
}
