import { useEffect, useState } from 'react';
import { CheckSquare, ArrowUpRight } from 'lucide-react';
import { Link } from 'react-router-dom';
import { api, message, dateTime, type Playbook, type FirmPlaybook } from '../lib/api';
import { Loading, ErrorNotice } from '../components/Feedback';
const explanations: Record<string, string> = {
    indemnity: 'Compares an explicit liability cap in months of fees with the chosen floor. An uncapped position or unclear units needs lawyer review.',
    renewal: 'Looks for automatic renewal and compares the non-renewal notice period with the chosen maximum.',
    law: 'Looks for an explicit governing-law clause and compares its jurisdiction with the matter’s accepted list.',
    breach: 'Locates security-incident notification language and compares an explicit deadline in hours with the chosen maximum. Deadlines stated in days require manual review.',
    confidentiality: 'Locates matching confidentiality language. The reviewer confirms obligations, scope, duration and exceptions.',
    termination: 'Locates matching termination language. The reviewer confirms termination rights, cure mechanisms and commercial adequacy.',
    assignment: 'Locates matching assignment language. The reviewer confirms consent requirements, permitted transfers and change-of-control treatment.',
    data_return: 'Locates matching data-return or deletion language. The reviewer confirms timing, backups and retention exceptions.',
};
export default function ChecklistsPage() {
    const [playbook, setPlaybook] = useState<Playbook | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');
    const [library, setLibrary] = useState<FirmPlaybook[]>([]);
    async function load() { setLoading(true); setError(''); try {
        const [catalog, saved] = await Promise.all([api<Playbook>('/api/playbook'), api<{playbooks: FirmPlaybook[]}>('/api/firm/playbooks')]);
        setPlaybook(catalog); setLibrary(saved.playbooks);
    }
    catch (error) {
        setError(message(error));
    }
    finally {
        setLoading(false);
    } }
    useEffect(() => { load(); }, []);
    return <div className="page-enter"><div className="page-heading"><div><p className="eyebrow">THE FIRM PLAYBOOK</p><h1>Checklists</h1><p className="muted">Consistent first-pass checks, with the policy you choose.</p></div><CheckSquare className="brass" size={30}/></div>
    {loading ? <Loading /> : error ? <ErrorNotice error={error} retry={load}/> : playbook && <><div className="notice policy-notice">These checks apply your firm’s chosen policy. Thresholds are not statutory rules, and every result requires lawyer review. Unsupported wording may need manual verification.</div>
      <section className="surface firm-library"><div className="section-heading"><h2>Saved firm playbooks</h2><span className="mini-label">{library.length} VERSIONS</span></div><p className="muted">Create numeric rules, preferred clauses and reference links in a matter’s Findings page. Save a version to this library, then load it into another matter.</p>{library.length ? library.slice().reverse().map(row => <article key={row.id} className="library-row"><h3>{row.policy.name} · v{row.version}</h3><p>{row.policy.enabled_checks.length} standard checks · {row.policy.custom_rules.length} custom rules</p><small>{row.author} · {dateTime(row.created_at)}</small>{row.policy.custom_rules.map(rule => <p key={rule.id}>{rule.name}: {rule.kind === 'semantic' ? 'Semantic clause expectation' : rule.kind === 'nda_term' ? `≤ ${rule.threshold} months` : `≥ ${rule.currency} ${rule.threshold.toLocaleString()}`} · {rule.applies_to === 'nda' ? 'NDA-labelled documents' : 'all documents'}</p>)}</article>) : <p className="muted">Your library is ready for its first saved playbook.</p>}<Link to="/matters" className="button subtle">Open a matter<ArrowUpRight size={16}/></Link></section>
      <section className="surface checklist-catalog"><div className="section-heading"><h2>{playbook.name}</h2><span className="mini-label">{playbook.items.length} SUPPORTED CHECKS</span></div>{playbook.items.map((item, index) => <article key={item.key} className="checklist-row"><span className="step-number">{String(index + 1).padStart(2, '0')}</span><div><h3>{item.item}</h3><p>{item.description || explanations[item.key]}</p><span className="check-threshold">{item.playbook}</span></div><CheckSquare size={17} className="brass"/></article>)}</section>
      <section className="surface preset-section"><div className="section-heading"><h2>Policy starting points</h2><span className="mini-label">CUSTOMIZE PER MATTER</span></div><p className="muted">Select a preset in a matter’s Findings page, then adjust the thresholds and enabled checks before saving.</p><div className="preset-list">{playbook.presets.map(preset => <article key={preset.id}><h3>{preset.name}</h3><dl><div><dt>Liability floor</dt><dd>{preset.policy.liability_floor_months} months of fees</dd></div><div><dt>Renewal notice</dt><dd>≤ {preset.policy.renewal_notice_days} days</dd></div><div><dt>Breach notice</dt><dd>≤ {preset.policy.breach_notice_hours} hours</dd></div><div><dt>Accepted laws</dt><dd>{preset.policy.accepted_laws.join(', ')}</dd></div><div><dt>Enabled checks</dt><dd>{preset.policy.enabled_checks.length}</dd></div></dl></article>)}</div><Link to="/matters" className="button subtle">Choose a matter<ArrowUpRight size={16}/></Link></section>
    </>}
  </div>;
}
