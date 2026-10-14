import { useEffect, useState, type FormEvent } from 'react';
import { SlidersHorizontal, ChevronDown, Save } from 'lucide-react';
import { api, json, message, type Playbook, type Policy, type Matter, type FirmPlaybook } from '../lib/api';
import CustomRulesEditor from './CustomRulesEditor';
import { useMatter } from '../pages/MatterLayout';
import { ErrorNotice, Loading } from './Feedback';
import { useSession } from '../lib/session';
export default function PolicyEditor() {
    const { matter, setMatter } = useMatter();
    const {user} = useSession();
    const readonly = user?.role === 'viewer' || !!matter.hibernated;
    const [expanded, setExpanded] = useState(false);
    const [playbook, setPlaybook] = useState<Playbook | null>(null);
    const [policy, setPolicy] = useState<Policy | null>(matter.policy || null);
    const [laws, setLaws] = useState((matter.policy?.accepted_laws || []).join(', '));
    const [error, setError] = useState('');
    const [saving, setSaving] = useState(false);
    const [loading, setLoading] = useState(false);
    const [saved, setSaved] = useState(false);
    const [library, setLibrary] = useState<FirmPlaybook[]>([]);
    const [libraryNotice, setLibraryNotice] = useState('');
    useEffect(() => { if (matter.policy) {
        setPolicy(matter.policy);
        setLaws(matter.policy.accepted_laws.join(', '));
    } }, [matter.policy]);
    useEffect(() => {
        if (!expanded || playbook)
            return;
        setLoading(true);
        Promise.all([api<Playbook>('/api/playbook'), api<{playbooks: FirmPlaybook[]}>('/api/firm/playbooks')]).then(([data, saved]) => { setPlaybook(data); setLibrary(saved.playbooks); if (!policy) {
            setPolicy(data.defaults);
            setLaws(data.defaults.accepted_laws.join(', '));
        } }).catch(error => setError(message(error))).finally(() => setLoading(false));
    }, [expanded]);
    function apply(next: Policy) { setPolicy({ ...next, enabled_checks: [...next.enabled_checks], accepted_laws: [...next.accepted_laws], custom_rules: (next.custom_rules || []).map(rule => ({...rule})) }); setLaws(next.accepted_laws.join(', ')); setSaved(false); setLibraryNotice(''); }
    async function save(event: FormEvent) {
        event.preventDefault();
        if (!policy || readonly)
            return;
        setSaving(true);
        setError('');
        setSaved(false);
        try {
            const next = { ...policy, accepted_laws: laws.split(',').map(law => law.trim()).filter(Boolean) };
            if (!next.enabled_checks.length && !next.custom_rules.length)
                throw new Error('Enable at least one check.');
            if (!next.accepted_laws.length)
                throw new Error('Enter at least one accepted governing law.');
            setMatter(await api<Matter>(`/api/matters/${matter.id}/playbook`, { method: 'PUT', body: json(next) }));
            setSaved(true);
        }
        catch (error) {
            setError(message(error));
        }
        finally {
            setSaving(false);
        }
    }
    async function saveTemplate() {
        if (!policy || readonly) return;
        setSaving(true); setError(''); setLibraryNotice('');
        try {
            const next = { ...policy, accepted_laws: laws.split(',').map(law => law.trim()).filter(Boolean) };
            const template = await api<FirmPlaybook>('/api/firm/playbooks', { method: 'POST', body: json(next) });
            setLibrary([...library, template]);
            setLibraryNotice(`Saved ${template.policy.name}, version ${template.version}, to your firm library. Save the matter policy to apply it here.`);
        } catch (error) { setError(message(error)); } finally { setSaving(false); }
    }
    return <section className="policy-editor surface"><button className="policy-toggle" onClick={() => setExpanded(!expanded)} aria-expanded={expanded}><SlidersHorizontal size={17}/><span>Matter playbook <strong>{matter.policy?.name || matter.playbook || 'Default policy'}</strong></span><ChevronDown size={18} className={expanded ? 'rotate' : ''}/></button>
    {expanded && <div className="policy-body"><p className="muted">These are your firm’s chosen review thresholds. They are not statutory requirements. Changing the policy makes an existing audit out of date.</p><ErrorNotice error={error}/>{readonly && <p className="field-help">Matter policy is read-only for this account or archived matter.</p>}
      {loading ? <Loading text="Loading playbook…"/> : policy && <form onSubmit={save} className="form-stack"><fieldset className="form-fields" disabled={readonly || saving}><div><label htmlFor="policy-preset">Start from a preset</label><select id="policy-preset" defaultValue="" onChange={e => { const preset = playbook?.presets.find(p => p.id === e.target.value); if (preset)
                apply(preset.policy); }}><option value="">Choose a preset</option>{playbook?.presets.map(preset => <option key={preset.id} value={preset.id}>{preset.name}</option>)}</select></div>
        <div><label htmlFor="firm-template">Load a saved firm playbook</label><select id="firm-template" value="" onChange={e => { const template = library.find(row => row.id === e.target.value); if (template) apply(template.policy); }}><option value="">Choose a saved version</option>{library.slice().reverse().map(row => <option key={row.id} value={row.id}>{row.policy.name} · v{row.version}</option>)}</select><p className="field-help">Each saved version is a snapshot in this firm account. Matter policies remain editable copies.</p></div>
        <div className="form-grid"><div><label htmlFor="policy-name">Policy name</label><input id="policy-name" required maxLength={100} value={policy.name} onChange={e => { setPolicy({ ...policy, name: e.target.value }); setSaved(false); }}/></div><div><label htmlFor="policy-laws">Accepted governing laws</label><input id="policy-laws" required value={laws} onChange={e => { setLaws(e.target.value); setSaved(false); }}/><p className="field-help">Separate jurisdictions with commas.</p></div></div>
        <div className="form-grid three">{[{ key: 'liability_floor_months', label: 'Liability floor (months of fees)' }, { key: 'renewal_notice_days', label: 'Maximum renewal notice (days)' }, { key: 'breach_notice_hours', label: 'Maximum breach notice (hours)' }].map(({ key, label }) => <div key={key}><label htmlFor={key}>{label}</label><input id={key} type="number" min="1" step="1" required value={policy[key as 'liability_floor_months']} onChange={e => { setPolicy({ ...policy, [key]: Number(e.target.value) }); setSaved(false); }}/></div>)}</div>
        <fieldset><legend>Enabled checks</legend><div className="check-options">{playbook?.items.map(item => <label key={item.key}><input type="checkbox" checked={policy.enabled_checks.includes(item.key)} onChange={e => { setPolicy({ ...policy, enabled_checks: e.target.checked ? [...policy.enabled_checks, item.key] : policy.enabled_checks.filter(key => key !== item.key) }); setSaved(false); }}/>{item.item}</label>)}</div></fieldset>
        <CustomRulesEditor rules={policy.custom_rules || []} onChange={rules => { setPolicy({...policy, custom_rules: rules}); setSaved(false); setLibraryNotice(''); }}/>
        <div className="inline-actions"><button className="button primary" disabled={saving || readonly}><Save size={16}/>{saving ? 'Saving…' : 'Save matter policy'}</button><button type="button" className="button subtle" disabled={saving || readonly} onClick={saveTemplate}>Save version to firm library</button>{saved && <span className="success-text" role="status">Policy saved. Run the audit to apply it.</span>}</div>{libraryNotice && <p className="success-text" role="status">{libraryNotice}</p>}
      </fieldset></form>}
    </div>}
  </section>;
}
