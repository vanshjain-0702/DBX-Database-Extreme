import { useState } from 'react';
import { Link } from 'react-router-dom';
import { ArrowUpRight, Play, Download, CheckCheck } from 'lucide-react';
import { useMatter } from './MatterLayout';
import { api, dateTime, downloadExport, message, findingsFor, type Matter, type ExportRecord } from '../lib/api';
import { ErrorNotice } from '../components/Feedback';
import { useSession } from '../lib/session';
export default function MatterDashboard() {
    const { matter, setMatter } = useMatter();
    const {user} = useSession();
    const readonly = user?.role === 'viewer' || !!matter.hibernated;
    const [busy, setBusy] = useState('');
    const [error, setError] = useState('');
    const findings = findingsFor(matter);
    const total = matter.review_total ?? findings.length;
    const reviewed = matter.reviewed ?? findings.filter(f => ['resolved', 'accepted_risk'].includes(f.review?.status || '')).length;
    async function run() { if (readonly) return; setBusy('audit'); setError(''); try {
        setMatter(await api<Matter>(`/api/matters/${matter.id}/audit`, { method: 'POST' }));
    }
    catch (error) {
        setError(message(error));
    }
    finally {
        setBusy('');
    } }
    async function exportMemo() { if (readonly) return; setBusy('export'); setError(''); try {
        const data = await api<{
            export: ExportRecord;
        }>(`/api/matters/${matter.id}/export`, { method: 'POST' });
        await downloadExport(data.export.id);
    }
    catch (error) {
        setError(message(error));
    }
    finally {
        setBusy('');
    } }
    return <div className="dashboard-layout"><div><ErrorNotice error={error}/>{matter.stale && <div className="notice warning">Documents or policy changed since the last run. Run a new audit before recording review decisions.</div>}
    <section className="surface audit-overview"><p className="eyebrow">AUDIT STATUS</p><div className="section-heading"><h2>{!matter.has_run ? 'Ready when your documents are.' : matter.stale ? 'Time for a fresh review.' : 'The evidence is ready for review.'}</h2><span className={`badge ${matter.stale ? 'missing' : matter.has_run ? 'clear' : 'neutral'}`}>{matter.stale ? 'Out of date' : matter.has_run ? 'Audit complete' : 'Not run'}</span></div><p className="muted">First-pass checks; lawyer review required.</p>
      <div className="metric-strip"><div><strong>{matter.doc_count || 0}</strong><span>documents</span></div><div><strong>{matter.has_run ? total : '—'}</strong><span>document checks</span></div><div><strong>{matter.has_run ? findings.filter(f => f.state === 'high').length : '—'}</strong><span>high risk</span></div><div><strong>{matter.has_run ? findings.filter(f => f.state === 'missing').length : '—'}</strong><span>missing clauses</span></div></div>
      <div className="inline-actions"><button className="button primary" onClick={run} disabled={!!busy || readonly || !matter.doc_count}><Play size={16}/>{busy === 'audit' ? 'Running audit…' : matter.has_run ? 'Run audit again' : 'Run audit'}</button><Link className="button subtle" to="../findings">Open findings<ArrowUpRight size={16}/></Link></div><p className="field-help">Last run: {dateTime(matter.ran_at)} · {matter.policy?.name || matter.playbook}</p>
    </section>
    <section className="surface next-steps"><div className="section-heading"><h2>Next actions</h2><span className="mini-label">A COMPLETE REVIEW</span></div><Link to="../documents"><span className="step-number">01</span><div><h3>Add or read contract documents</h3><p>Read source versions, verify extracted text and check the contract wording.</p></div><ArrowUpRight size={18}/></Link><Link to="../findings"><span className="step-number">02</span><div><h3>Check the evidence, record a decision</h3><p>Review each document against your policy and leave a rationale.</p></div><ArrowUpRight size={18}/></Link><button onClick={exportMemo} disabled={!!busy || readonly || !matter.has_run || matter.stale}><span className="step-number">03</span><div><h3>{busy === 'export' ? 'Preparing review memo…' : 'Export the review memo'}</h3><p>Download findings, citations and the recorded review trail as Markdown.</p></div><Download size={18}/></button></section>
    </div><aside className="surface review-summary"><CheckCheck size={23} className="brass"/><p className="eyebrow">HUMAN REVIEW</p><h2>{reviewed}<span> / {total}</span></h2><p className="muted">Document checks with a recorded final decision.</p><progress value={reviewed} max={total || 1} aria-label="Human review progress"/><dl><div><dt>Outstanding</dt><dd>{matter.review_open ?? total - reviewed}</dd></div><div><dt>Resolved</dt><dd>{findings.filter(f => f.review?.status === 'resolved').length}</dd></div><div><dt>Accepted risk</dt><dd>{findings.filter(f => f.review?.status === 'accepted_risk').length}</dd></div></dl><p className="field-help">A check meeting the playbook still needs a lawyer’s review.</p><Link to="../findings" className="text-button">Continue review <ArrowUpRight size={15}/></Link></aside>
  </div>;
}
