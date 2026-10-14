import { useEffect, useRef, useState, type FormEvent } from 'react';
import { FileText, Plus, Upload, BookOpen, Download, Languages, GitCompareArrows } from 'lucide-react';
import { useMatter } from './MatterLayout';
import { api, json, message, dateTime, downloadFile, type DocumentContent, type Extraction } from '../lib/api';
import { useSession } from '../lib/session';
import { ErrorNotice, Loading, EmptyState } from '../components/Feedback';
interface Comparison {before: unknown; after: unknown; changes: {op: string; before: string; after: string}[]}
export default function MatterDocuments() {
    const { matter, refreshMatter } = useMatter();
    const {user, capabilities} = useSession();
    const readonly = user?.role === 'viewer' || !!matter.hibernated;
    const [name, setName] = useState('');
    const [text, setText] = useState('');
    const [busy, setBusy] = useState('');
    const [error, setError] = useState('');
    const [notice, setNotice] = useState('');
    const [selected, setSelected] = useState('');
    const [source, setSource] = useState<DocumentContent | null>(null);
    const [sourceLoading, setSourceLoading] = useState(false);
    const [sourceError, setSourceError] = useState('');
    const [reload, setReload] = useState(0);
    const [pending, setPending] = useState<Extraction | null>(null);
    const [reviewed, setReviewed] = useState(false);
    const [useOcr, setUseOcr] = useState(false);
    const [supersedes, setSupersedes] = useState('');
    const [language, setLanguage] = useState('English');
    const [glossary, setGlossary] = useState('');
    const [other, setOther] = useState('');
    const [comparison, setComparison] = useState<Comparison | null>(null);
    const draftKey = `jurix-extraction-${matter.id}`;
    const comparisonRequest = useRef(0);
    useEffect(() => {
        let active = true;
        const extractionId = sessionStorage.getItem(draftKey);
        if (extractionId) api<Extraction>(`/api/matters/${matter.id}/documents/extractions/${encodeURIComponent(extractionId)}`).then(extraction => { if (active) setDraft(extraction); }).catch(error => { if (active) setError(`Could not restore the extraction preview. ${message(error)}`); });
        return () => { active = false; };
    }, [matter.id]);
    useEffect(() => {
        comparisonRequest.current++;
        if (!selected) { setSource(null); setSourceLoading(false); setComparison(null); return; }
        let active = true; setSource(null); setSourceLoading(true); setSourceError(''); setComparison(null); setOther('');
        api<DocumentContent>(`/api/matters/${matter.id}/document?doc_id=${encodeURIComponent(selected)}`).then(data => { if (active) setSource(data); }).catch(error => { if (active) setSourceError(message(error)); }).finally(() => { if (active) setSourceLoading(false); });
        return () => { active = false; };
    }, [selected, matter.id, reload]);
    function setDraft(extraction: Extraction) {
        setPending(extraction); setName(extraction.name); setText(extraction.text); setReviewed(false);
        sessionStorage.setItem(draftKey, extraction.id);
    }
    function clearDraft() {
        setPending(null); setName(''); setText(''); setReviewed(false); setSupersedes(''); sessionStorage.removeItem(draftKey);
    }
    async function upload(file?: File) {
        if (!file) return;
        setBusy('upload'); setError(''); setNotice('');
        try {
            if (!/\.(txt|md|pdf|docx|png|jpe?g|tiff?)$/i.test(file.name)) throw new Error('Choose a TXT, Markdown, PDF, DOCX or supported image.');
            if (file.size > 20 * 1024 * 1024) throw new Error('Files must be no larger than 20 MB.');
            const body = new FormData(); body.append('file', file); body.append('use_ocr', String(useOcr));
            setDraft(await api<Extraction>(`/api/matters/${matter.id}/documents/upload`, {method: 'POST', body}));
            setNotice('Extraction ready. Check every page and correct the text before adding it to the matter.');
        } catch (error) { setError(message(error)); } finally { setBusy(''); }
    }
    async function ingest(event: FormEvent) {
        event.preventDefault(); setBusy('document'); setError(''); setNotice('');
        try {
            if (pending && !reviewed) throw new Error('Confirm you have reviewed and corrected the extracted text.');
            const body = {doc_name: name.trim(), text: text.trim(), ...(supersedes ? {supersedes} : {})};
            const doc = await api<{id: string; document?: {id: string}}>(pending ? `/api/matters/${matter.id}/documents/extractions/${pending.id}/commit` : `/api/matters/${matter.id}/documents`, {method: 'POST', body: json(body)});
            await refreshMatter(); setSelected(doc.document?.id || doc.id); clearDraft();
            setNotice('Immutable document version added. Run a fresh audit; previous decisions remain in history.');
        } catch (error) { setError(message(error)); } finally { setBusy(''); }
    }
    async function sample(kind: 'msa' | 'dpa') {
        setBusy(kind); setError(''); setNotice('');
        try { const doc = await api<{id: string}>(`/api/matters/${matter.id}/sample`, {method: 'POST', body: json({kind})}); await refreshMatter(); setSelected(doc.id); setNotice('Sample added. Run the audit to review its findings.'); }
        catch (error) { setError(message(error)); } finally { setBusy(''); }
    }
    async function translate(event: FormEvent) {
        event.preventDefault(); setBusy('translate'); setError(''); setNotice('');
        try {
            const entries = glossary.split('\n').map(line => line.trim()).filter(Boolean).map(line => {
                const separator = line.indexOf('=');
                if (separator < 1 || !line.slice(separator + 1).trim()) throw new Error('Use one glossary term per line: source = target.');
                return {source: line.slice(0, separator).trim(), target: line.slice(separator + 1).trim()};
            });
            setDraft(await api<Extraction>(`/api/matters/${matter.id}/documents/${selected}/translate`, {method: 'POST', body: json({target_language: language.trim(), glossary: entries})}));
            setSupersedes(''); setNotice('Translation draft ready. Review legal terms and source meaning before committing.');
        } catch (error) { setError(message(error)); } finally { setBusy(''); }
    }
    async function redline() {
        setBusy('redline'); setError('');
        try { await downloadFile(`/api/matters/${matter.id}/documents/${selected}/redline`, {method: 'POST'}, 'jurix-redline.docx'); setNotice('Word redline downloaded. It contains accepted proposals as text-derived tracked changes for counsel review.'); }
        catch (error) { setError(message(error)); } finally { setBusy(''); }
    }
    async function compare() {
        const requestId = ++comparisonRequest.current;
        setBusy('compare'); setError('');
        try { const data = await api<Comparison>(`/api/matters/${matter.id}/documents/${selected}/compare?other_id=${encodeURIComponent(other)}`); if (requestId === comparisonRequest.current) setComparison(data); }
        catch (error) { if (requestId === comparisonRequest.current) setError(message(error)); } finally { setBusy(''); }
    }
    const accepted = (matter.document_runs?.find(run => run.doc_id === selected)?.items || []).filter(item => item.review?.suggestion_decision === 'accepted').length;
    return <div className="documents-page">
        <ErrorNotice error={error}/>{notice && <p className="notice success" role="status">{notice}</p>}
        <div className="documents-layout">
        {!readonly && <section className="surface document-ingest"><div className="section-heading"><div><p className="eyebrow">SOURCE INTAKE</p><h2>{pending ? 'Verify extracted text' : 'Add a contract'}</h2></div><FileText size={20} className="brass"/></div>
            <p className="muted">Upload a document or paste its text. Review the source before adding an immutable version.</p>
            {!pending && <div className="upload-zone"><label htmlFor="document-file"><Upload size={20}/>Choose a source file</label><input id="document-file" type="file" accept=".txt,.md,.pdf,.docx,.png,.jpg,.jpeg,.tif,.tiff" disabled={!!busy} onChange={event => { void upload(event.target.files?.[0]); event.target.value = ''; }}/><p className="field-help">TXT, MD, PDF, DOCX or image · maximum 20 MB</p><label className="check-label"><input type="checkbox" checked={useOcr} disabled={!!busy || !capabilities?.ocr?.available || !matter.external_processing} onChange={event => setUseOcr(event.target.checked)}/>Use OCR for scanned pages</label>{!capabilities ? <p className="field-help">Provider availability is unverified. Check Firm & connection.</p> : !capabilities.ocr?.available ? <p className="field-help">OCR provider is not configured. Check Firm & connection.</p> : !matter.external_processing && <p className="field-help">Enable external processing above to use the OCR provider.</p>}</div>}
            {busy === 'upload' && <Loading text="Extracting source text…"/>}
            {pending && <div className="extraction-meta"><span className="badge missing">Needs reviewer verification</span><p className="field-help">{pending.method} · {pending.pages?.length || 0} source pages</p>{pending.warnings?.map((warning,index) => <p key={index} className="notice warning">{warning}</p>)}<details><summary>Page extraction preview</summary>{pending.pages?.map(page => <section className="source-chunk" key={page.number}><span className="mini-label">PAGE {page.number}</span><p>{page.text}</p></section>)}</details>{pending.has_original !== false && <button type="button" className="text-button" onClick={() => void downloadFile(`/api/matters/${matter.id}/documents/extractions/${pending.id}/original`, {}, pending.name).catch(error => setError(message(error)))}><Download size={14}/>Download original source</button>}</div>}
            <form onSubmit={ingest} className="form-stack"><div><label htmlFor="document-name">Document name</label><input id="document-name" required maxLength={200} value={name} onChange={event => setName(event.target.value)} placeholder="Services agreement — draft 01"/></div><div><label htmlFor="document-text">{pending ? 'Correct extracted text' : 'Contract text'}</label><textarea id="document-text" required rows={12} value={text} onChange={event => {setText(event.target.value); setReviewed(false);}} placeholder="Paste the source contract text here…"/></div><div><label htmlFor="document-supersedes">Version of an existing document</label><select id="document-supersedes" value={supersedes} onChange={event => setSupersedes(event.target.value)}><option value="">New document</option>{matter.documents?.map(doc => <option key={doc.id} value={doc.id}>{doc.name}{doc.version ? ` · v${doc.version}` : ''}</option>)}</select></div>{pending && <label className="check-label"><input type="checkbox" checked={reviewed} onChange={event => setReviewed(event.target.checked)}/>I reviewed the source pages and corrected the extracted text.</label>}<div className="inline-actions"><button className="button primary" disabled={!!busy || !name.trim() || !text.trim() || !!pending && !reviewed}><Plus size={16}/>{busy === 'document' ? 'Adding document…' : pending ? 'Confirm & add version' : 'Add document'}</button>{pending && <button className="button subtle" type="button" disabled={!!busy} onClick={clearDraft}>Discard preview</button>}</div></form>
            {!pending && <div className="sample-actions"><span className="mini-label">TRY A SAMPLE</span><button className="button subtle" onClick={() => sample('msa')} disabled={!!busy}>Sample MSA</button><button className="button subtle" onClick={() => sample('dpa')} disabled={!!busy}>Sample DPA</button></div>}
        </section>}
        <section className="surface document-library"><div className="section-heading"><div><p className="eyebrow">MATTER SOURCES</p><h2>Document library</h2></div><span className="mini-label">{matter.documents?.length || 0} VERSIONS</span></div>
            {matter.documents?.length ? <ul className="document-list">{matter.documents.map(doc => <li key={doc.id}><button className={selected === doc.id ? 'selected' : ''} aria-pressed={selected === doc.id} onClick={() => setSelected(doc.id)}><FileText size={18}/><span><strong>{doc.name}{doc.version ? ` · v${doc.version}` : ''}</strong><small>{doc.chunk_count} passages · {dateTime(doc.ingested_at)}</small></span><BookOpen size={16}/></button></li>)}</ul> : <EmptyState title="No documents yet" text="Add a source contract or try a sample to begin."/>}
            {selected && <div className="document-reader">{sourceLoading ? <Loading text="Reading source document…"/> : sourceError ? <ErrorNotice error={sourceError} retry={() => setReload(value => value + 1)}/> : source && <><h3>{source.name}</h3><div className="source-metadata"><span className="mini-label">{source.version ? `VERSION ${source.version}` : 'IMMUTABLE SOURCE'}{source.method ? ` · ${source.method}` : ''}</span>{source.source_sha256 && <p className="field-help mono">Source SHA-256: {source.source_sha256}</p>}{source.extraction_id && source.has_original !== false && <button className="text-button" onClick={() => void downloadFile(`/api/matters/${matter.id}/documents/extractions/${source.extraction_id}/original`, {}, source.name).catch(error => setError(message(error)))}><Download size={14}/>Original file</button>}</div><details open><summary>Read source text</summary><pre className="original-source">{source.text || source.chunks?.map(chunk => chunk.text).join('\n\n')}</pre></details>{source.pages?.length ? <details><summary>Source page spans</summary>{source.pages.map(page => <p key={page.number} className="field-help">Page {page.number}: characters {page.start}–{page.end}</p>)}</details> : null}
                <div className="document-tools"><h4>Versions & counsel exports</h4><div><label htmlFor="compare-document">Compare this version with</label><select id="compare-document" value={other} onChange={event => {comparisonRequest.current++; setOther(event.target.value); setComparison(null);}}><option value="">Choose another version</option>{matter.documents?.filter(doc => doc.id !== selected).map(doc => <option key={doc.id} value={doc.id}>{doc.name}{doc.version ? ` · v${doc.version}` : ''}</option>)}</select></div><button className="button subtle" disabled={!!busy || !other} onClick={compare}><GitCompareArrows size={15}/>Compare versions</button>{comparison && <div className="version-comparison"><div className="comparison-heading"><span>Compared version</span><span>Selected version</span></div>{comparison.changes.length ? comparison.changes.map((change,index) => <div key={index} className={`comparison-row ${change.op}`}><pre>{change.before || '—'}</pre><pre>{change.after || '—'}</pre></div>) : <p className="muted">No text differences found.</p>}</div>}
                {!readonly && <><button className="button subtle" disabled={!!busy || !accepted || matter.stale} onClick={redline}><Download size={15}/>{busy === 'redline' ? 'Preparing…' : 'Word redline'}<span className="mini-label">{accepted} accepted</span></button><p className="field-help">Accepted proposals only. Text-derived tracked changes; counsel checks formatting and source wording.</p></>}
                </div>
                {!readonly && <details className="translation-tools"><summary><Languages size={16}/>Prepare a translation draft</summary><form className="form-stack" onSubmit={translate}><p className="field-help">Translation uses the configured external provider. Review terminology and legal meaning against the original before committing.</p><div><label htmlFor="target-language">Target language</label><input id="target-language" required maxLength={80} value={language} onChange={event => setLanguage(event.target.value)}/></div><div><label htmlFor="translation-glossary">Firm glossary (optional)</label><textarea id="translation-glossary" rows={3} value={glossary} onChange={event => setGlossary(event.target.value)} placeholder="Indemnity = your preferred translated term"/><p className="field-help">One source = target term per line.</p></div><button className="button primary" disabled={!!busy || !capabilities?.translation?.available || !matter.external_processing || !!pending}><Languages size={15}/>{busy === 'translate' ? 'Translating…' : 'Create translation draft'}</button>{!capabilities ? <p className="field-help">Provider availability is unverified. Check Firm & connection.</p> : !capabilities.translation?.available ? <p className="field-help">Translation provider is not configured. Check Firm & connection.</p> : !matter.external_processing && <p className="field-help">Enable external processing for this matter first.</p>}{pending && <p className="field-help">Finish or discard the existing extraction preview first.</p>}</form></details>}
            </>}</div>}
        </section>
        </div>
    </div>;
}
