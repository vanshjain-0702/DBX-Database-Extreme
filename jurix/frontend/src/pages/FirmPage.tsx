import { useEffect, useState, type FormEvent } from 'react';
import { RefreshCw, Building2, Radio, Users, Copy, Check } from 'lucide-react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { api, json, message, type Health, type User } from '../lib/api';
import { useSession } from '../lib/session';
import { Loading, ErrorNotice } from '../components/Feedback';
interface Member {email: string; role: User['role']; active: boolean}
interface Invitation {email: string; role: User['role']; expires_at?: number; accepted?: boolean}
export default function FirmPage() {
    const {user, capabilities, reload} = useSession();
    const admin = user?.role === 'admin';
    const [params] = useSearchParams();
    const navigate = useNavigate();
    const [health, setHealth] = useState<Health | null>(null);
    const [members, setMembers] = useState<Member[]>([]);
    const [invitations, setInvitations] = useState<Invitation[]>([]);
    const [error, setError] = useState('');
    const [healthError, setHealthError] = useState('');
    const [loading, setLoading] = useState(true);
    const [teamLoaded, setTeamLoaded] = useState(false);
    const [busy, setBusy] = useState('');
    const [checking, setChecking] = useState(false);
    const [checkedAt, setCheckedAt] = useState('');
    const [email, setEmail] = useState('');
    const [role, setRole] = useState<User['role']>('reviewer');
    const [inviteLink, setInviteLink] = useState('');
    const [expires, setExpires] = useState('');
    const [copied, setCopied] = useState(false);
    const [notice, setNotice] = useState('');
    const [inviteToken, setInviteToken] = useState(params.get('invite') || '');
    async function check() {
        setChecking(true); setHealthError('');
        try {setHealth(await api<Health>('/api/readiness')); setCheckedAt(new Date().toLocaleTimeString());}
        catch (error) {setHealth(null); setHealthError(message(error));} finally {setChecking(false);}
    }
    async function load() {
        setLoading(true); setError('');
        try {const data = await api<{members: Member[]; invitations: Invitation[]}>('/api/firm/members'); setMembers(data.members); setInvitations(data.invitations || []); setTeamLoaded(true);}
        catch (error) {setError(message(error));} finally {setLoading(false);}
    }
    useEffect(() => {void load(); void check();}, []);
    async function invite(event: FormEvent) {
        event.preventDefault(); setBusy('invite'); setError(''); setInviteLink(''); setCopied(false);
        try {
            const data = await api<{token: string; expires_at: number}>('/api/firm/invitations', {method: 'POST', body: json({email: email.trim(), role})});
            const location = new URL(window.location.href); location.hash = `/login?invite=${encodeURIComponent(data.token)}`;
            setInviteLink(location.toString()); setExpires(new Date(data.expires_at * 1000).toLocaleString()); setEmail(''); await load();
        } catch (error) {setError(message(error));} finally {setBusy('');}
    }
    async function update(member: Member, change: Partial<Member>) {
        setBusy(member.email); setError(''); setNotice('');
        try {await api(`/api/firm/members/${encodeURIComponent(member.email)}`, {method: 'PATCH', body: json(change)}); await load(); if (member.email === user?.email) await reload(); setNotice('Team access updated.');}
        catch (error) {setError(message(error));} finally {setBusy('');}
    }
    async function accept(event: FormEvent) {
        event.preventDefault(); setBusy('accept'); setError('');
        try {await api('/api/firm/invitations/accept', {method: 'POST', body: json({token: inviteToken.trim()})}); await reload(); navigate('/login?joined=1', {replace: true});}
        catch (error) {setError(message(error));} finally {setBusy('');}
    }
    async function copy() {
        try {await navigator.clipboard.writeText(inviteLink); setCopied(true);}
        catch {setError('Clipboard access is unavailable. Select and copy the invitation link below.');}
    }
    const lastAdmin = (member: Member) => member.active && member.role === 'admin' && members.filter(row => row.active && row.role === 'admin').length === 1;
    return <div className="page-enter"><div className="page-heading"><div><p className="eyebrow">WORKSPACE ADMINISTRATION</p><h1>Firm & connection</h1><p className="muted">Team access, provider availability and storage status.</p></div><Building2 className="brass" size={28}/></div><ErrorNotice error={error}/>{notice && <p className="notice success" role="status">{notice}</p>}
        <div className="firm-layout"><section className="surface firm-details"><p className="eyebrow">FIRM ACCOUNT</p><h2>{user?.firm_name || 'Firm account'}</h2><dl><div><dt>Signed in as</dt><dd>{user?.name}</dd></div><div><dt>Work email</dt><dd>{user?.email}</dd></div><div><dt>Your role</dt><dd><span className="badge neutral">{user?.role}</span></dd></div><div><dt>Firm code</dt><dd className="mono">{user?.firm_code || 'Not available'}</dd></div></dl><p className="field-help">Viewers can read sources and saved reviews. Reviewers manage matter work. Administrators manage team access.</p></section><section className="surface connection-details"><div className="section-heading"><h2><Radio size={20}/>Live connection</h2><button className="button subtle" onClick={check} disabled={checking}><RefreshCw size={15}/>{checking ? 'Checking…' : 'Check now'}</button></div><ErrorNotice error={healthError}/><div className="service-row"><span>Jurix API</span><span className={`badge ${checking ? 'neutral' : health ? 'clear' : 'neutral'}`}>{checking ? 'Checking' : health ? 'Available' : 'Unverified'}</span></div><div className="service-row"><span>DBX storage</span><span className={`badge ${checking || !health ? 'neutral' : health.dbx?.ok ? 'clear' : 'missing'}`}>{checking ? 'Checking' : !health ? 'Unverified' : health.dbx?.ok ? 'Connected' : 'Unavailable'}</span></div>{health?.dbx?.detail && <p className="notice warning">{health.dbx.detail}</p>}<p className="field-help">Last checked: {checkedAt || 'Waiting for response'}</p></section></div>
        <section className="surface provider-settings"><div className="section-heading"><h2>Processing providers</h2><span className="mini-label">SERVER CONFIGURATION</span></div><p className="muted">Credentials stay on the server. Provider availability reports configuration; counsel still verifies every output.</p><div className="provider-list">{[{key: 'llm', label: 'Semantic clause review', setup: 'Configure the language-model provider and model on the JURIX server.'}, {key: 'ocr', label: 'Scanned-document OCR', setup: 'Configure the OCR provider on the JURIX server.'}, {key: 'translation', label: 'Translation drafts', setup: 'Configure the translation provider and model on the JURIX server.'}, {key: 'dictation', label: 'Dictation drafts', setup: 'Configure the audio transcription provider on the JURIX server.'}].map(provider => <div className="provider-row" key={provider.key}><div><strong>{provider.label}</strong><p className="field-help">{capabilities?.[provider.key as keyof typeof capabilities]?.available ? 'Enable external processing separately for each matter before use.' : provider.setup}</p></div><span className={`badge ${capabilities?.[provider.key as keyof typeof capabilities]?.available ? 'clear' : 'neutral'}`}>{capabilities ? capabilities[provider.key as keyof typeof capabilities]?.available ? 'Configured' : 'Unavailable' : 'Not checked'}</span></div>)}</div></section>
        <section className="surface team-settings"><div className="section-heading"><h2><Users size={20}/>Team access</h2><span className="mini-label">{loading ? 'LOADING TEAM' : !teamLoaded ? 'TEAM UNAVAILABLE' : `${members.length} MEMBERS`}</span></div>{loading ? <Loading text="Loading team…"/> : teamLoaded && <div className="table-scroll"><table className="team-table"><thead><tr><th scope="col">Work email</th><th scope="col">Role</th><th scope="col">Access</th></tr></thead><tbody>{members.map(member => <tr key={member.email}><th scope="row">{member.email}{member.email === user?.email && <small> · You</small>}</th><td>{admin ? <><label className="sr-only" htmlFor={`member-role-${member.email}`}>Role for {member.email}</label><select id={`member-role-${member.email}`} value={member.role} disabled={!!busy || lastAdmin(member)} onChange={event => void update(member, {role: event.target.value as User['role']})}><option value="admin">Administrator</option><option value="reviewer">Reviewer</option><option value="viewer">Viewer</option></select></> : member.role}</td><td>{admin ? <button className="button subtle" disabled={!!busy || lastAdmin(member)} onClick={() => void update(member, {active: !member.active})}>{member.active ? 'Deactivate' : 'Reactivate'}</button> : <span className={`badge ${member.active ? 'clear' : 'neutral'}`}>{member.active ? 'Active' : 'Inactive'}</span>}{lastAdmin(member) && <p className="field-help">Last active administrator</p>}</td></tr>)}</tbody></table></div>}
            {admin && teamLoaded && <form className="form-stack team-invite" onSubmit={invite}><h3>Invite a colleague</h3><p className="muted">Create a one-time invitation link and share it directly. JURIX does not send invitation email.</p><div className="form-grid"><div><label htmlFor="invite-email">Invited work email</label><input id="invite-email" type="email" required value={email} onChange={event => setEmail(event.target.value)}/></div><div><label htmlFor="invite-role">Role</label><select id="invite-role" value={role} onChange={event => setRole(event.target.value as User['role'])}><option value="reviewer">Reviewer</option><option value="viewer">Viewer</option><option value="admin">Administrator</option></select></div></div><button className="button primary" disabled={!!busy}>{busy === 'invite' ? 'Creating…' : 'Create invitation link'}</button>{inviteLink && <div className="invitation-result"><label htmlFor="invitation-link">One-time invitation link</label><input id="invitation-link" readOnly value={inviteLink} onFocus={event => event.target.select()}/><p className="field-help">Expires {expires}. Copy now; this link is only shown once.</p><button type="button" className="button subtle" onClick={copy}>{copied ? <Check size={15}/> : <Copy size={15}/>} {copied ? 'Copied' : 'Copy link'}</button></div>}</form>}
            {invitations.length > 0 && <details className="pending-invitations"><summary>Invitation history · {invitations.length}</summary>{invitations.map((invitation,index) => <p className="field-help" key={index}>{invitation.email} · {invitation.role} · {invitation.accepted ? 'Accepted' : 'Pending / expires ' + (invitation.expires_at ? new Date(invitation.expires_at * 1000).toLocaleDateString() : 'as issued')}</p>)}</details>}
        </section>
        <details className="surface join-firm" open={!!params.get('invite')}><summary>Join a firm with an invitation</summary><form className="form-stack" onSubmit={accept}><p className="field-help">Use the token from your invitation link. Joining changes your firm membership and requires a new sign-in.</p><div><label htmlFor="accept-invite">Invitation token</label><input id="accept-invite" required value={inviteToken} onChange={event => setInviteToken(event.target.value)}/></div><button className="button subtle" disabled={!!busy || !inviteToken.trim()}>Accept invitation</button></form></details>
    </div>;
}
