import { useState, type FormEvent } from 'react';
import { Navigate, useNavigate, useSearchParams } from 'react-router-dom';
import { Scale, ArrowRight, Loader2 } from 'lucide-react';
import { useSession } from '../lib/session';
import { api, json, message } from '../lib/api';
import { ErrorNotice } from '../components/Feedback';
export default function LoginPage() {
  const [params] = useSearchParams();
  const inviteToken = params.get('invite') || '';
  const [registering, setRegistering] = useState(!!inviteToken);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [firm, setFirm] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const navigate = useNavigate();
  const { user, loading, reload } = useSession();
  if (!loading && user) return <Navigate to={inviteToken ? `/firm?invite=${encodeURIComponent(inviteToken)}` : '/matters'} replace />;
  async function submit(event: FormEvent) {
    event.preventDefault();
    setError('');
    setBusy(true);
    try {
      await api(registering ? '/api/register' : '/api/login', { method: 'POST', body: json({ email: email.trim(), password, ...(registering ? { firm_name: firm.trim(), ...(inviteToken ? { invite_token: inviteToken } : {}) } : {}) }) }, false);
      localStorage.removeItem('jurix_token');
      await reload();
      navigate(!registering && inviteToken ? `/firm?invite=${encodeURIComponent(inviteToken)}` : '/matters');
    }
    catch (error) {
      setError(message(error));
    }
    finally {
      setBusy(false);
    }
  }
  return <div className="login-page">
    <div className="login-backdrop"><img src="/img/desk.jpg" alt="" /></div>
    <div className="login-brand"><Scale size={28} /> JURIX</div>
    <div className="login-composition"><div className="login-intro"><p className="eyebrow">THE REVIEWER’S WORKSPACE</p><h1>Every clause.<br />{' '}A clearer view.</h1><p>Bring the contract, the evidence and your firm’s playbook into one considered review.</p><span className="intro-rule" /></div>
      <section className="login-panel" aria-labelledby="login-heading"><p className="eyebrow">FIRM ACCESS</p><h2 id="login-heading">{registering ? 'Establish your workspace' : 'Welcome to chambers'}</h2><p className="muted">{registering ? 'Create an account for your firm.' : 'Sign in to continue your contract review.'}</p>
        <form onSubmit={submit} className="form-stack"><ErrorNotice error={error} />
          {params.get('joined') && <p className="notice success">Invitation accepted. Sign in again to access your new firm.</p>}{inviteToken && <p className="notice">Your invitation will connect this account to the inviting firm.</p>}
          {registering && !inviteToken && <div><label htmlFor="firm-name">Firm name</label><input id="firm-name" value={firm} onChange={event => setFirm(event.target.value)} required autoComplete="organization" /></div>}
          <div><label htmlFor="login-email">Work email</label><input id="login-email" type="email" value={email} onChange={event => setEmail(event.target.value)} required autoComplete="username" placeholder="you@firm.com" /></div>
          <div><label htmlFor="login-password">Password</label><input id="login-password" type="password" value={password} onChange={event => setPassword(event.target.value)} required minLength={registering ? 12 : undefined} autoComplete={registering ? 'new-password' : 'current-password'} />{registering && <p className="field-help">At least 12 characters.</p>}</div>
          <button className="button primary full-width" disabled={busy || loading}>{busy ? <Loader2 className="spin" size={18} /> : <ArrowRight size={18} />}{busy ? 'Please wait…' : registering ? inviteToken ? 'Join invited firm' : 'Create firm account' : 'Enter workspace'}</button>
        </form>
        <button className="text-button login-switch" type="button" disabled={busy} onClick={() => { setRegistering(!registering); setError(''); }}>{registering ? 'Already have an account? Sign in' : 'New to Jurix? Create an account'}</button>
      </section></div>
    <p className="login-footer">Contract diligence, guided by your judgment.</p>
  </div>;
}
