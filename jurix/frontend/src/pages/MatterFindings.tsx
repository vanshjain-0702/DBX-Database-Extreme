import { useEffect, useMemo, useRef, useState, type FormEvent } from 'react';
import { Search, Play, Download, FileText, ArrowUpRight, Check, MessageSquare, ChevronDown } from 'lucide-react';
import { Link } from 'react-router-dom';
import { useMatter } from './MatterLayout';
import PolicyEditor from '../components/PolicyEditor';
import DictationDraft from '../components/DictationDraft';
import { api, json, message, dateTime, findingsFor, severityLabel, reviewLabel, downloadExport, type Matter, type Finding, type DocumentContent, type ReviewStatus, type ExportRecord } from '../lib/api';
import { ErrorNotice, Loading, EmptyState } from '../components/Feedback';
import { useSession } from '../lib/session';
function EvidenceText({ text, quote }: {
    text: string;
    quote: string;
}) {
    const index = quote ? text.indexOf(quote) : -1;
    return <p>{index < 0 ? text : <>{text.slice(0, index)}<mark>{quote}</mark>{text.slice(index + quote.length)}</>}</p>;
}
function AnchoredSource({text, anchor}: {text: string; anchor: Finding['anchor']}) {
    if (!anchor) return <pre className="original-source">{text}</pre>;
    // API coordinates count Unicode code points; JS string offsets count UTF-16 units.
    const characters = Array.from(text);
    return <pre className="original-source">{characters.slice(0, anchor.start).join('')}<mark>{characters.slice(anchor.start, anchor.end).join('')}</mark>{characters.slice(anchor.end).join('')}</pre>;
}
export default function MatterFindings() {
    const { matter, setMatter, refreshMatter } = useMatter();
    const {user} = useSession();
    const readonly = user?.role === 'viewer' || !!matter.hibernated;
    const findings = useMemo(() => findingsFor(matter), [matter]);
    const [severity, setSeverity] = useState('all');
    const [reviewFilter, setReviewFilter] = useState('all');
    const [query, setQuery] = useState('');
    const [selectedId, setSelectedId] = useState('');
    const [busy, setBusy] = useState('');
    const [error, setError] = useState('');
    const [notice, setNotice] = useState('');
    const [reviewStatus, setReviewStatus] = useState<ReviewStatus>('open');
    const [proposalDecision, setProposalDecision] = useState<'pending' | 'accepted' | 'rejected'>('pending');
    const [rationale, setRationale] = useState('');
    const [note, setNote] = useState('');
    const [source, setSource] = useState<DocumentContent | null>(null);
    const [sourceLoading, setSourceLoading] = useState(false);
    const [sourceError, setSourceError] = useState('');
    const [sourceReload, setSourceReload] = useState(0);
    const [showAllSource, setShowAllSource] = useState(false);
    const evidenceHeading = useRef<HTMLHeadingElement>(null);
    const sourceViewer = useRef<HTMLDivElement>(null);
    const matrixScroll = useRef<HTMLDivElement>(null);
    const [matrixOverflow, setMatrixOverflow] = useState(false);
    const shown = findings.filter(finding => (severity === 'all' || finding.state === severity) && (reviewFilter === 'all' || (finding.review?.status || 'open') === reviewFilter) && [finding.item, finding.doc_name, finding.quote, finding.finding].join(' ').toLowerCase().includes(query.toLowerCase()));
    const selected = shown.find(finding => finding.id === selectedId) || shown[0] || null;
    useEffect(() => { setReviewStatus(selected?.review?.status || 'open'); setProposalDecision(selected?.review?.suggestion_decision || 'pending'); setRationale(selected?.review?.note || ''); setNote(''); setShowAllSource(false); }, [selected?.id, selected?.review?.updated_at]);
    useEffect(() => {
        if (!selected?.doc_id) {
            setSource(null);
            setSourceLoading(false);
            setSourceError('');
            return;
        }
        let active = true;
        setSourceLoading(true);
        setSourceError('');
        setSource(null);
        api<DocumentContent>(`/api/matters/${matter.id}/document?doc_id=${encodeURIComponent(selected.doc_id)}`).then(document => { if (active)
            setSource(document); }).catch(error => { if (active)
            setSourceError(message(error)); }).finally(() => { if (active)
            setSourceLoading(false); });
        return () => { active = false; };
    }, [selected?.doc_id, matter.id, sourceReload]);
    async function runAudit() {
        if (readonly) return;
        setBusy('audit');
        setError('');
        setNotice('');
        try {
            setMatter(await api<Matter>(`/api/matters/${matter.id}/audit`, { method: 'POST' }));
            setSelectedId('');
            setNotice('Audit complete. Review the evidence and record your decisions.');
        }
        catch (error) {
            setError(message(error));
        }
        finally {
            setBusy('');
        }
    }
    async function saveReview(event: FormEvent) {
        event.preventDefault();
        if (!selected || readonly || matter.stale)
            return;
        setBusy('review');
        setError('');
        setNotice('');
        try {
            if ((['resolved', 'accepted_risk'].includes(reviewStatus) || proposalDecision !== 'pending') && !rationale.trim())
                throw new Error('Add a rationale for a resolved or accepted-risk decision.');
            setMatter(await api<Matter>(`/api/matters/${matter.id}/findings/${encodeURIComponent(selected.id)}/review`, { method: 'PATCH', body: json({ status: reviewStatus, note: rationale.trim(), suggestion_decision: proposalDecision }) }));
            setNotice('Review decision recorded.');
        }
        catch (error) {
            setError(message(error));
        }
        finally {
            setBusy('');
        }
    }
    async function saveNote(event: FormEvent) {
        event.preventDefault();
        if (!selected || !note.trim() || readonly)
            return;
        setBusy('note');
        setError('');
        setNotice('');
        try {
            await api(`/api/matters/${matter.id}/notes`, { method: 'POST', body: json({ item_key: selected.id, text: note.trim() }) });
            await refreshMatter();
            setNote('');
            setNotice('Note added to the review record.');
        }
        catch (error) {
            setError(message(error));
        }
        finally {
            setBusy('');
        }
    }
    async function exportMemo() {
        if (readonly) return;
        setBusy('export');
        setError('');
        try {
            const result = await api<{
                export: ExportRecord;
            }>(`/api/matters/${matter.id}/export`, { method: 'POST' });
            await downloadExport(result.export.id);
            setNotice('Review memo prepared. Your browser will handle the download; a copy is available in Exports.');
        }
        catch (error) {
            setError(message(error));
        }
        finally {
            setBusy('');
        }
    }
    async function exportProposals() {
        if (readonly) return;
        setBusy('proposals'); setError('');
        try {
            const result = await api<{export: ExportRecord}>(`/api/matters/${matter.id}/export-proposals`, {method: 'POST'});
            await downloadExport(result.export.id);
            setNotice('Accepted clause comparisons prepared for download and saved in Exports. Counsel applies the wording to the agreement.');
        } catch (error) { setError(message(error)); } finally { setBusy(''); }
    }
    function viewCitedLines() {
        const details = sourceViewer.current?.closest('details');
        if (details) details.open = true;
        setShowAllSource(true);
        requestAnimationFrame(() => {
            const target = sourceViewer.current?.querySelector('mark');
            target?.scrollIntoView({block: 'center', behavior: 'auto'});
            sourceViewer.current?.focus({preventScroll: true});
        });
    }
    function selectFinding(finding: Finding, fromMatrix = false) {
        if (fromMatrix) {
            setSeverity('all');
            setReviewFilter('all');
            setQuery('');
        }
        setSelectedId(finding.id);
        setError('');
        setNotice('');
        if (window.matchMedia('(max-width: 760px)').matches) {
            requestAnimationFrame(() => {
                evidenceHeading.current?.focus({ preventScroll: true });
                evidenceHeading.current?.scrollIntoView({
                    block: 'start',
                    behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth',
                });
            });
        }
    }
    useEffect(() => {
        const element = matrixScroll.current;
        if (!element) return;
        const update = () => setMatrixOverflow(element.scrollWidth > element.clientWidth + 1);
        update();
        const observer = new ResizeObserver(update);
        observer.observe(element);
        return () => observer.disconnect();
    }, [findings.length]);
    const columns = [...new Map(findings.map(finding => [finding.key, { key: finding.key, name: finding.item }])).values()];
    const runs = matter.document_runs || [];
    const citedChunk = source?.chunks.find(chunk => chunk.id === selected?.chunk_id);
    const sourceChunks = showAllSource ? source?.chunks : citedChunk ? [citedChunk] : [];
    const notes = (matter.notes || []).filter(row => row.item_key === selected?.id || row.item_key === selected?.key);
    const activities = (matter.activity || []).filter(row => row.item_id === selected?.id).slice().reverse();
    const acceptedProposals = findings.filter(item => item.proposed_text && item.review?.suggestion_decision === 'accepted').length;
    return <div className="findings-page">
    <div className="workbench-heading"><div><h2>Contract diligence</h2><p className="muted">First-pass checks; lawyer review required.</p></div><div className="inline-actions"><button className="button subtle" onClick={exportMemo} disabled={!!busy || readonly || !matter.has_run || matter.stale}><Download size={16}/>{busy === 'export' ? 'Exporting…' : 'Export memo'}</button><button className="button primary" onClick={runAudit} disabled={!!busy || readonly || !matter.doc_count}><Play size={16}/>{busy === 'audit' ? 'Running audit…' : 'Run audit'}</button></div></div>
    <ErrorNotice error={error}/>{notice && <div className="notice success" role="status">{notice}</div>}{matter.stale && <div className="notice warning">This audit is out of date. Run it again to include changed documents or policy. Review decisions are paused until then.</div>}
    {readonly && <p className="notice">{matter.hibernated ? 'This matter is archived. Saved sources and reviews remain available.' : 'Viewer access: read sources and saved decisions. Ask a reviewer to record changes.'}</p>}
    <PolicyEditor />
    {runs.filter(run => run.scope_note).map(run => <p key={run.doc_id} className="muted scope-note">{run.doc_name}: {run.scope_note}.</p>)}
    {acceptedProposals > 0 && <div className="proposal-export inline-actions"><span className="muted">{acceptedProposals} accepted firm clause proposal{acceptedProposals === 1 ? '' : 's'}</span><button className="button subtle" disabled={!!busy || readonly || matter.stale} onClick={exportProposals}><Download size={16}/>Export clause comparisons</button></div>}
    {findings.length > 0 && <section className="surface matrix-section"><div className="section-heading"><div><p className="eyebrow">DOCUMENT × CHECK</p><h3>Diligence matrix</h3></div><span className="mini-label">SELECT A CELL TO REVIEW</span></div><div className="table-scroll" ref={matrixScroll} tabIndex={0} role="region" aria-label="Document diligence matrix; scroll horizontally to compare all checks"><table className="diligence-matrix"><thead><tr><th scope="col">Document</th>{columns.map(column => <th scope="col" key={column.key}>{column.name}</th>)}</tr></thead><tbody>{runs.map(run => <tr key={run.doc_id}><th scope="row">{run.doc_name}</th>{columns.map(column => { const finding = run.items.find(item => item.key === column.key); return <td key={column.key}>{finding ? <button className={`matrix-cell ${finding.state} ${selected?.id === finding.id ? 'selected' : ''}`} onClick={() => selectFinding(finding, true)} aria-label={`${run.doc_name}: ${finding.item}, ${severityLabel(finding.state)}, ${reviewLabel(finding.review?.status || 'open')}`} title={`${severityLabel(finding.state)} · ${reviewLabel(finding.review?.status || 'open')}`}><span className="status-dot"/>{finding.state === 'clear' ? 'Clear' : finding.state === 'missing' ? 'Missing' : finding.state === 'high' ? 'High' : 'Medium'}{['resolved', 'accepted_risk'].includes(finding.review?.status || '') && <Check size={12}/>}</button> : <span className="muted">—</span>}</td>; })}</tr>)}</tbody></table></div>{matrixOverflow && <p className="matrix-scroll-cue">Scroll horizontally to see all checks <span aria-hidden="true">→</span></p>}<div className="matrix-legend"><span><i className="high"/>High risk</span><span><i className="medium"/>Medium risk</span><span><i className="missing"/>Missing clause</span><span><i className="clear"/>Meets check</span><span><Check size={12}/>Final decision recorded</span></div></section>}
    {!findings.length ? <section className="surface"><EmptyState title={!matter.doc_count ? 'Start with a contract' : 'Ready for a first pass'} text={!matter.doc_count ? 'Add document text or a sample agreement, then run the audit.' : 'Run the audit to compare each document with your selected playbook.'}>{!matter.doc_count && <Link className="button primary" to="../documents">Add documents<ArrowUpRight size={16}/></Link>}</EmptyState></section> : <div className="review-workbench">
      <section className="surface findings-list"><div className="section-heading"><h3>Findings</h3><span className="mini-label">{shown.length} / {findings.length}</span></div>
        <div className="finding-filters"><div className="search-field"><Search size={15}/><label className="sr-only" htmlFor="finding-search">Search findings</label><input id="finding-search" value={query} onChange={e => setQuery(e.target.value)} placeholder="Search findings or evidence"/></div><div className="form-grid"><div><label className="sr-only" htmlFor="severity-filter">Severity</label><select id="severity-filter" value={severity} onChange={e => setSeverity(e.target.value)}><option value="all">All severities</option><option value="high">High risk</option><option value="medium">Medium risk</option><option value="missing">Missing</option><option value="clear">Meets check</option></select></div><div><label className="sr-only" htmlFor="review-filter">Review status</label><select id="review-filter" value={reviewFilter} onChange={e => setReviewFilter(e.target.value)}><option value="all">All review states</option><option value="open">Open</option><option value="in_review">In review</option><option value="resolved">Resolved</option><option value="accepted_risk">Accepted risk</option></select></div></div></div>
        <div className="finding-rows">{shown.length ? shown.map(finding => <button key={finding.id} onClick={() => selectFinding(finding)} className={`finding-row ${selected?.id === finding.id ? 'selected' : ''}`} aria-pressed={selected?.id === finding.id}><div className="finding-row-top"><span className={`badge ${finding.state}`}>{severityLabel(finding.state)}</span><span className="finding-number">{String(finding.number || columns.findIndex(c => c.key === finding.key) + 1).padStart(2, '0')}</span></div><h4>{finding.item}</h4><p><FileText size={12}/>{finding.doc_name}</p><span className="review-state">{reviewLabel(finding.review?.status || 'open')}</span></button>) : <EmptyState title="No matching findings" text="Adjust the search or filters to see more checks."/>}</div>
      </section>
      <section className="surface evidence-panel" aria-label="Finding evidence and review">{selected ? <>
        <div className="evidence-header"><div><p className="eyebrow">EVIDENCE & DECISION</p><h3 ref={evidenceHeading} tabIndex={-1} className="selected-evidence-heading">{selected.item}</h3></div><span className={`badge ${selected.state}`}>{severityLabel(selected.state)}</span></div>
        <div className="evidence-body"><div className="finding-conclusion"><span className="mini-label">FIRST-PASS FINDING</span><p>{selected.finding}</p></div>
          <div className="playbook-rule"><span className="mini-label">PLAYBOOK EXPECTATION</span><p>{selected.playbook}</p></div>
          <p className="field-help">Method: {selected.method || 'Deterministic first-pass text check'}. This finding applies a firm standard; counsel verifies its legal effect.</p>
          {selected.quote && !selected.absent ? <blockquote>{selected.quote}<footer><FileText size={13}/>{selected.doc_name} · {selected.citation || selected.mark || 'Source passage'}</footer></blockquote> : <div className="absence-evidence"><FileText size={18}/><div><strong>No matching clause located</strong><p>The first-pass check did not locate supporting text in {selected.doc_name}. Read the document to confirm.</p></div></div>}
          {selected.suggestion && <div className="fallback-suggestion"><span className="mini-label">SUGGESTED FOLLOW-UP</span><p>{selected.suggestion}</p><small>Drafting aid for lawyer consideration.</small></div>}
          {selected.authority && <div className="authority-reference"><a className="text-button" href={selected.authority.url} target="_blank" rel="noopener noreferrer">{selected.authority.title}<ArrowUpRight size={14}/></a><p className="field-help">Firm-supplied reference. Counsel verifies currency and applicability.</p></div>}
          {selected.anchor && <button className="text-button" disabled={!source || sourceLoading} onClick={viewCitedLines}>View cited lines {selected.anchor.line_start}–{selected.anchor.line_end}<ArrowUpRight size={14}/></button>}
          {selected.proposed_text && <div className="clause-proposal"><span className="mini-label">FIRM PREFERRED CLAUSE</span><p>{selected.proposed_text}</p><small>Record acceptance or rejection below. Accepted wording is exported as a comparison for counsel to apply.</small></div>}
          <details className="source-details" open><summary><FileText size={16}/>Source document <ChevronDown size={15}/></summary><div className="source-viewer" ref={sourceViewer} tabIndex={-1}>
            {sourceLoading ? <Loading text="Loading source…"/> : sourceError ? <ErrorNotice error={sourceError} retry={() => setSourceReload(value => value + 1)}/> : source && <><div className="source-title"><strong>{source.name}</strong><span>{source.chunks.length} passages</span></div>{showAllSource && source.text ? <AnchoredSource text={source.text} anchor={selected.anchor}/> : sourceChunks?.length ? sourceChunks.map(chunk => <section key={chunk.id} className="source-chunk"><span className="mini-label">PASSAGE {chunk.index + 1}{chunk.id === selected.chunk_id ? ' · CITED EVIDENCE' : ''}</span><EvidenceText text={chunk.text} quote={selected.quote}/></section>) : <p className="muted">No cited passage for this check. Open the full document to verify the clause.</p>}<button className="text-button" onClick={() => setShowAllSource(!showAllSource)}>{showAllSource ? 'Show cited passage only' : 'Read full document'}<ArrowUpRight size={14}/></button></>}
          </div></details>
          <form className="review-form form-stack" onSubmit={saveReview}><div className="section-heading"><h4>Reviewer decision</h4>{selected.review?.author && <span className="field-help">{selected.review.author}</span>}</div><div><label htmlFor="review-status">Review status</label><select id="review-status" value={reviewStatus} onChange={e => setReviewStatus(e.target.value as ReviewStatus)} disabled={readonly || !!busy || matter.stale}><option value="open">Open</option><option value="in_review">In review</option><option value="resolved">Resolved</option><option value="accepted_risk">Accepted risk</option></select></div>
          {selected.proposed_text && <div><label htmlFor="proposal-decision">Firm clause proposal decision</label><select id="proposal-decision" value={proposalDecision} onChange={e => setProposalDecision(e.target.value as typeof proposalDecision)} disabled={readonly || !!busy || matter.stale}><option value="pending">Pending review</option><option value="accepted">Accept for clause comparison export</option><option value="rejected">Reject proposal</option></select></div>}
          <div><label htmlFor="review-rationale">Decision rationale{['resolved', 'accepted_risk'].includes(reviewStatus) || proposalDecision !== 'pending' ? ' (required)' : ''}</label><textarea id="review-rationale" rows={3} maxLength={10000} value={rationale} onChange={e => setRationale(e.target.value)} disabled={readonly || !!busy || matter.stale} required={['resolved', 'accepted_risk'].includes(reviewStatus) || proposalDecision !== 'pending'} placeholder="Explain the evidence, proposed change or reason for accepting the risk."/></div><div className="inline-actions"><button className="button primary" disabled={!!busy || readonly || matter.stale}><Check size={16}/>{busy === 'review' ? 'Saving…' : 'Save decision'}</button>{selected.review?.updated_at && <span className="field-help">Updated {dateTime(selected.review.updated_at)}</span>}</div></form>
          <div className="review-notes"><h4><MessageSquare size={16}/>Review notes</h4>{notes.map(row => <article key={row.id} className="note"><p>{row.text}</p><small>{row.author || 'Reviewer'} · {dateTime(row.created_at)}</small></article>)}<form onSubmit={saveNote} className="form-stack"><label htmlFor="finding-note" className="sr-only">Add a review note</label><textarea id="finding-note" rows={2} disabled={readonly || !!busy} value={note} onChange={e => setNote(e.target.value)} placeholder="Add a note to this document check…"/><button className="button subtle" disabled={!!busy || readonly || !note.trim()}>{busy === 'note' ? 'Adding…' : 'Add note'}</button></form>{!readonly && <DictationDraft key={`${matter.id}:${selected.id}`} disabled={!!busy} onConfirm={draft => setNote(previous => [previous, draft].filter(Boolean).join('\n\n'))}/>}</div>
          {activities.length > 0 && <details className="review-history"><summary>Decision history · {activities.length}</summary>{activities.map(activity => <article key={activity.id}><strong>{reviewLabel(activity.status)}{activity.suggestion_decision && activity.suggestion_decision !== 'pending' ? ` · Proposal ${activity.suggestion_decision}` : ''}</strong><p>{activity.note || 'Status updated'}</p><small>{activity.author} · {dateTime(activity.created_at)}{activity.policy ? ` · ${activity.policy.name}` : ''}</small>{activity.evidence && <details><summary>Evidence at this decision</summary><p>{activity.evidence.doc_name} · {activity.evidence.state}</p><blockquote>{activity.evidence.quote || 'No matching provision located'}</blockquote>{activity.evidence.proposed_text && <p>Proposed wording: {activity.evidence.proposed_text}</p>}</details>}</article>)}</details>}
        </div>
      </> : <EmptyState title="Select a finding" text="Choose a finding or matrix cell to inspect its source and record a decision."/>}</section>
    </div>}
  </div>;
}
