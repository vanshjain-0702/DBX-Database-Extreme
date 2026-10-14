import { useEffect, useState, type FormEvent } from 'react';
import { Download, Play } from 'lucide-react';
import { useMatter } from './MatterLayout';
import { api, json, message, dateTime, downloadExport, type BillingPolicy, type BillingRun, type ExportRecord } from '../lib/api';
import { ErrorNotice, Loading } from '../components/Feedback';
import { useSession } from '../lib/session';

const SAMPLE = `date,timekeeper,hours,task_code,activity_code,narrative
2026-10-01,Associate,2.5,L120,A104,Worked on case
2026-10-01,Partner,3,L110,A104,Reviewed witness statement; drafted response to opposing counsel
2026-10-02,Associate,1.8,L140,A103,Scheduling client meeting for the coming week
2026-10-02,Associate,0.5,L120,A104,Reviewed limitation of liability clause for supplier negotiation`;

export default function MatterBilling() {
    const { matter } = useMatter();
    const {user} = useSession();
    const readonly = user?.role === 'viewer' || !!matter.hibernated;
    const [runs, setRuns] = useState<BillingRun[]>([]);
    const [selectedId, setSelectedId] = useState('');
    const [policy, setPolicy] = useState<BillingPolicy | null>(null);
    const [csv, setCsv] = useState('');
    const [routine, setRoutine] = useState('');
    const [busy, setBusy] = useState('load');
    const [error, setError] = useState('');
    const [notice, setNotice] = useState('');
    const [reload, setReload] = useState(0);
    const selected = runs.find(run => run.id === selectedId) || runs[0];
    useEffect(() => {
        let active = true;
        setBusy('load'); setError('');
        api<{runs: BillingRun[]; defaults: BillingPolicy}>(`/api/matters/${matter.id}/billing`).then(data => {
            if (!active) return;
            setRuns(data.runs); setPolicy(data.defaults); setRoutine(data.defaults.routine_terms.join(', ')); setSelectedId('');
        }).catch(error => { if (active) setError(message(error)); }).finally(() => { if (active) setBusy(''); });
        return () => { active = false; };
    }, [matter.id, reload]);
    async function audit(event: FormEvent) {
        event.preventDefault(); if (!policy || readonly) return;
        setBusy('audit'); setError(''); setNotice('');
        try {
            const nextPolicy = {...policy, routine_terms: routine.split(',').map(term => term.trim()).filter(Boolean)};
            const run = await api<BillingRun>(`/api/matters/${matter.id}/billing/audit`, {method: 'POST', body: json({csv_text: csv, policy: nextPolicy})});
            setRuns(previous => [run, ...previous]); setSelectedId(run.id);
            setNotice('Pre-bill review saved with a snapshot of the entries and client guidelines. Billing partner review is required.');
        } catch (error) { setError(message(error)); } finally { setBusy(''); }
    }
    async function importFile(file?: File) {
        if (!file) return;
        setError(''); setNotice('');
        try {
            if (!file.name.toLowerCase().endsWith('.csv')) throw new Error('Choose a UTF-8 CSV file.');
            if (file.size > 1000000) throw new Error('CSV files must be at most 1 MB.');
            setCsv(await file.text());
        } catch (error) { setError(message(error)); }
    }
    async function exportReport() {
        if (!selected || readonly) return;
        setBusy('export'); setError('');
        try {
            const result = await api<{export: ExportRecord}>(`/api/matters/${matter.id}/billing/${selected.id}/export`, {method: 'POST'});
            await downloadExport(result.export.id);
            setNotice('Billing review prepared for download and saved in Exports.');
        } catch (error) { setError(message(error)); } finally { setBusy(''); }
    }
    return <div className="billing-page">
        <div className="workbench-heading"><div><h2>Pre-bill review</h2><p className="muted">Review narratives and ABA 1997 Litigation / Activity codes against the client's guidelines.</p></div>{selected && !readonly && <button className="button subtle" onClick={exportReport} disabled={!!busy}><Download size={16}/>Export billing review</button>}</div>
        {readonly && <p className="notice">Read-only access. Saved pre-bill reviews appear below; a reviewer can prepare a new snapshot.</p>}
        <ErrorNotice error={error} retry={!policy ? () => setReload(v => v + 1) : undefined}/>{notice && <div className="notice success" role="status">{notice}</div>}
        {busy === 'load' ? <Loading text="Loading billing reviews…"/> : policy && !readonly && <form className="surface billing-input form-stack" onSubmit={audit}>
            <div className="section-heading"><h3>Client billing guidelines</h3><span className="mini-label">CUSTOMIZE BEFORE REVIEW</span></div>
            <p className="muted">These checks flag entries for partner review. Thresholds are your guidelines, and a flag does not determine that a fee is improper. Other UTBMS code sets and LEDES invoice formats require a separate validator.</p>
            <div><label htmlFor="billing-name">Guideline name</label><input id="billing-name" required maxLength={100} value={policy.name} onChange={e => setPolicy({...policy, name: e.target.value})}/></div>
            <div className="form-grid three">{[{key: 'min_narrative_words', label: 'Minimum narrative words', max: 100, step: 1}, {key: 'max_entry_hours', label: 'Maximum hours per entry', max: 24, step: 0.1}, {key: 'routine_max_hours', label: 'Routine task hours threshold', max: 24, step: 0.1}].map(field => <div key={field.key}><label htmlFor={`billing-${field.key}`}>{field.label}</label><input id={`billing-${field.key}`} required type="number" min={field.step} max={field.max} step={field.step} value={policy[field.key as 'max_entry_hours']} onChange={e => setPolicy({...policy, [field.key]: Number(e.target.value)})}/></div>)}</div>
            <div className="check-options">{[{key: 'require_task_code', label: 'Require task codes'}, {key: 'require_activity_code', label: 'Require activity codes'}, {key: 'flag_block_billing', label: 'Flag possible block billing'}].map(field => <label key={field.key}><input type="checkbox" checked={policy[field.key as 'require_task_code']} onChange={e => setPolicy({...policy, [field.key]: e.target.checked})}/>{field.label}</label>)}</div>
            <div><label htmlFor="billing-routine">Routine task phrases</label><input id="billing-routine" value={routine} onChange={e => setRoutine(e.target.value)}/><p className="field-help">Separate phrases with commas. Clear this field to disable routine phrase checks.</p></div>
            <div className="section-heading"><h3>Time entries</h3><button type="button" className="text-button" disabled={!!busy} onClick={() => { setCsv(SAMPLE); setNotice('Sample entries loaded. Review the client guidelines, then run the checks.'); }}>Load sample entries</button></div>
            <div><label htmlFor="billing-file">Import a UTF-8 CSV (up to 1 MB, 500 entries)</label><input id="billing-file" type="file" accept=".csv,text/csv" disabled={!!busy} onChange={e => { void importFile(e.target.files?.[0]); e.target.value = ''; }}/></div>
            <div><label htmlFor="billing-csv">CSV entries</label><textarea className="billing-csv" id="billing-csv" rows={8} required maxLength={1000000} value={csv} onChange={e => setCsv(e.target.value)} placeholder="date,timekeeper,hours,task_code,activity_code,narrative"/><p className="field-help">Use the header above in that order. Dates use YYYY-MM-DD; quote narratives containing commas or line breaks. Correct entries here and rerun to retain a new review snapshot.</p></div>
            <button className="button primary" disabled={!!busy || !csv.trim()}><Play size={16}/>{busy === 'audit' ? 'Reviewing…' : 'Review time entries'}</button>
        </form>}
        {!busy && !error && !selected && <section className="surface"><p className="muted">No saved billing reviews for this matter.</p></section>}
        {busy !== 'load' && selected && <section className="surface billing-results"><div className="section-heading"><div><p className="eyebrow">SAVED REVIEW</p><h3>{selected.flagged_entries} of {selected.entries.length} entries flagged</h3><p className="muted">{selected.total_hours} hours · {selected.flag_count} flags · {selected.policy.name}</p></div></div>
            <div><label htmlFor="billing-history">Review snapshot</label><select id="billing-history" value={selected.id} onChange={e => setSelectedId(e.target.value)}>{runs.map(run => <option key={run.id} value={run.id}>{dateTime(run.created_at)} · {run.policy.name} · {run.flagged_entries} flagged</option>)}</select></div>
            <p className="field-help">Reviewed by {selected.author} · {dateTime(selected.created_at)}. {selected.method}</p>
            <details><summary>Guidelines used for this snapshot</summary><p>{selected.policy.min_narrative_words} minimum words; {selected.policy.max_entry_hours} maximum hours; {selected.policy.routine_max_hours} routine hours. Routine phrases: {selected.policy.routine_terms.join(', ') || 'disabled'}.</p><p>Task codes {selected.policy.require_task_code ? 'required' : 'optional'}; activity codes {selected.policy.require_activity_code ? 'required' : 'optional'}; block billing check {selected.policy.flag_block_billing ? 'enabled' : 'disabled'}.</p></details>
            {selected.entries.map(entry => <article key={entry.row} className="billing-entry"><div className="section-heading"><h4>Entry {entry.row - 1} · {entry.timekeeper}</h4><span className={`badge ${entry.flags.length ? 'medium' : 'clear'}`}>{entry.flags.length ? 'Needs review' : 'No flags'}</span></div><p className="field-help">{entry.date} · {entry.hours} hours · {entry.task_code || 'No task code'} / {entry.activity_code || 'No activity code'}</p><blockquote>{entry.narrative}</blockquote>{entry.flags.map(flag => <div className="billing-flag" key={flag.code}><strong>{flag.description}</strong><p>{flag.next_step}</p></div>)}</article>)}
            <div className="inline-actions">{selected.sources.map(source => <a key={source.url} className="text-button" href={source.url} target="_blank" rel="noopener noreferrer">{source.title}</a>)}</div>
        </section>}
    </div>;
}
