import type { CustomRule } from '../lib/api';

export default function CustomRulesEditor({ rules, onChange }: { rules: CustomRule[]; onChange: (rules: CustomRule[]) => void }) {
    function update(index: number, change: Partial<CustomRule>) {
        onChange(rules.map((rule, i) => i === index ? { ...rule, ...change } : rule));
    }
    function add(kind: CustomRule['kind']) {
        const nda = kind === 'nda_term';
        onChange([...rules, {
            id: `rule_${crypto.randomUUID().slice(0, 8)}`, kind, name: kind === 'semantic' ? 'Custom clause expectation' : nda ? 'NDA agreement term' : 'Fixed monetary liability floor',
            threshold: kind === 'semantic' ? 1 : nda ? 24 : 50000, instructions: '', currency: 'USD', applies_to: nda ? 'nda' : 'all',
            preferred_text: '', authority_title: '', authority_url: '',
        }]);
    }
    return <fieldset className="custom-rules"><legend>Custom firm rules</legend>
        <p className="muted">Set numeric thresholds or describe a clause expectation for semantic review. Semantic rules require a configured language model and matter permission for external processing. Every finding needs counsel review.</p>
        {rules.map((rule, index) => <article key={rule.id} className="custom-rule">
            <div className="section-heading"><h4>Rule {index + 1} · {rule.kind === 'semantic' ? 'Semantic clause review' : rule.kind === 'nda_term' ? 'Agreement term' : 'Monetary liability cap'}</h4><button type="button" className="text-button" onClick={() => onChange(rules.filter((_, i) => i !== index))} aria-label={`Remove ${rule.name}`}>Remove</button></div>
            <div className="form-grid"><div><label htmlFor={`rule-name-${rule.id}`}>Rule name</label><input id={`rule-name-${rule.id}`} required maxLength={100} value={rule.name} onChange={e => update(index, { name: e.target.value })}/></div>
                <div><label htmlFor={`rule-scope-${rule.id}`}>Apply to</label><select id={`rule-scope-${rule.id}`} value={rule.applies_to} onChange={e => update(index, { applies_to: e.target.value as CustomRule['applies_to'] })}><option value="all">All documents</option><option value="nda">Documents labelled as NDAs</option></select></div></div>
            {rule.kind === 'semantic' ? <div><label htmlFor={`rule-instructions-${rule.id}`}>Clause expectation and review instructions</label><textarea id={`rule-instructions-${rule.id}`} rows={4} required maxLength={8000} value={rule.instructions || ''} onChange={event => update(index, {instructions: event.target.value})} placeholder="Describe the obligations, exceptions and conditions counsel expects. Findings must quote exact source wording."/><p className="field-help">The reviewer checks source quotations and firm references. Model-generated analysis is a drafting aid.</p></div> : <div className="form-grid"><div><label htmlFor={`rule-limit-${rule.id}`}>{rule.kind === 'nda_term' ? 'Maximum agreement term (months)' : 'Minimum liability cap (amount)'}</label><input id={`rule-limit-${rule.id}`} type="number" min={1} max={rule.kind === 'nda_term' ? 120 : 1000000000} step={1} required value={rule.threshold} onChange={e => update(index, { threshold: Number(e.target.value) })}/></div>
                {rule.kind === 'liability_amount' && <div><label htmlFor={`rule-currency-${rule.id}`}>Currency</label><select id={`rule-currency-${rule.id}`} value={rule.currency} onChange={e => update(index, { currency: e.target.value as CustomRule['currency'] })}>{['USD', 'INR', 'EUR', 'GBP'].map(currency => <option key={currency}>{currency}</option>)}</select></div>}</div>}
            <div><label htmlFor={`rule-text-${rule.id}`}>Firm preferred clause (optional)</label><textarea id={`rule-text-${rule.id}`} rows={3} maxLength={4000} value={rule.preferred_text} onChange={e => update(index, { preferred_text: e.target.value })} placeholder="Enter counsel-approved wording to propose when this rule flags a passage."/><p className="field-help">Proposals require a reviewer decision and rationale. Acceptance records the proposal for export; counsel applies edits to the agreement.</p></div>
            <div className="form-grid"><div><label htmlFor={`rule-authority-${rule.id}`}>Reference title (optional)</label><input id={`rule-authority-${rule.id}`} maxLength={200} required={!!rule.authority_url} value={rule.authority_title} onChange={e => update(index, { authority_title: e.target.value })} placeholder="Firm policy or relevant legal authority"/></div>
                <div><label htmlFor={`rule-url-${rule.id}`}>Reference link (HTTPS)</label><input id={`rule-url-${rule.id}`} type="url" pattern="https://.*" maxLength={2000} required={!!rule.authority_title} value={rule.authority_url} onChange={e => update(index, { authority_url: e.target.value })}/></div></div>
            <p className="field-help">Firm-supplied references are review aids. Counsel verifies their accuracy, currency and applicability.</p>
        </article>)}
        <div className="inline-actions"><button className="button subtle" type="button" disabled={rules.length >= 20} onClick={() => add('semantic')}>Add semantic rule</button><button className="button subtle" type="button" disabled={rules.length >= 20} onClick={() => add('nda_term')}>Add NDA term rule</button><button className="button subtle" type="button" disabled={rules.length >= 20} onClick={() => add('liability_amount')}>Add monetary cap rule</button></div>
    </fieldset>;
}
