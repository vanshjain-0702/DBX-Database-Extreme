export type Severity = 'high' | 'medium' | 'missing' | 'clear';
export type ReviewStatus = 'open' | 'in_review' | 'resolved' | 'accepted_risk';
export interface Review {
    status: ReviewStatus;
    note?: string;
    author?: string;
    updated_at?: number;
    suggestion_decision?: 'pending' | 'accepted' | 'rejected';
}
export interface Finding {
    id: string;
    key: string;
    number?: number;
    item: string;
    playbook: string;
    state: Severity;
    finding: string;
    quote: string;
    mark?: string;
    absent?: boolean;
    chunk_id?: string;
    chunk_index?: number;
    citation?: string;
    doc_name: string;
    doc_id: string;
    via?: string;
    review?: Review;
    suggestion?: string;
    proposed_text?: string;
    anchor?: { start: number; end: number; line_start: number; line_end: number } | null;
    authority?: { title: string; url: string; provenance: string } | null;
    method?: string;
}
export interface CustomRule {
    id: string;
    name: string;
    kind: 'nda_term' | 'liability_amount' | 'semantic';
    instructions?: string;
    threshold: number;
    currency: 'USD' | 'INR' | 'EUR' | 'GBP';
    applies_to: 'all' | 'nda';
    preferred_text: string;
    authority_title: string;
    authority_url: string;
}
export interface Policy {
    name: string;
    liability_floor_months: number;
    renewal_notice_days: number;
    breach_notice_hours: number;
    accepted_laws: string[];
    enabled_checks: string[];
    custom_rules: CustomRule[];
}
export interface FirmPlaybook { id: string; version: number; policy: Policy; author: string; created_at: number }
export interface BillingPolicy {
    name: string; require_task_code: boolean; require_activity_code: boolean; flag_block_billing: boolean;
    min_narrative_words: number; max_entry_hours: number; routine_max_hours: number; routine_terms: string[];
}
export interface BillingRun {
    id: string; created_at: number; author: string; policy: BillingPolicy; code_set: string; method: string;
    total_hours: number; flagged_entries: number; flag_count: number;
    sources: { title: string; url: string }[];
    entries: {
        row: number; date: string; timekeeper: string; hours: number; task_code: string; activity_code: string; narrative: string;
        flags: { code: string; description: string; next_step: string }[]
    }[];
}
export interface DocumentRecord {
    id: string;
    name: string;
    chunk_count: number;
    ingested_at: number;
    version?: number;
    supersedes?: string;
    source_sha256?: string;
    method?: string;
}
export interface DocumentContent {
    has_original?: boolean;
    id: string;
    name: string;
    text: string;
    version?: number;
    supersedes?: string;
    source_sha256?: string;
    text_sha256?: string;
    extraction_id?: string;
    method?: string;
    pages?: { number: number; text: string; start: number; end: number }[];
    chunks: {
        id: string;
        index: number;
        text: string;
    }[];
}
export interface Note {
    id: string;
    item_key: string;
    text: string;
    author?: string;
    created_at?: number;
}
export interface Activity {
    id: string;
    item_id: string;
    status: ReviewStatus;
    note: string;
    author: string;
    created_at: number;
    fingerprint?: string;
    policy?: Policy;
    evidence?: { quote?: string; proposed_text?: string; doc_name?: string; authority?: Finding['authority']; state?: Severity };
    suggestion_decision?: string;
}
export interface Matter {
    id: string;
    name: string;
    matter: string;
    tenant_id: string;
    reviewer_name: string;
    doc_count: number;
    has_run: boolean;
    ran_at?: number;
    stale: boolean;
    status: string;
    high: number;
    medium: number;
    missing: number;
    clear: number;
    open: number;
    reviewed?: number;
    review_total?: number;
    review_open?: number;
    documents?: DocumentRecord[];
    items?: Finding[];
    document_runs?: {
        doc_id: string;
        doc_name: string;
        items: Finding[];
        scope_note?: string;
    }[];
    notes?: Note[];
    activity?: Activity[];
    policy?: Policy;
    playbook?: string;
    hibernated?: boolean;
    external_processing?: boolean;
}
export interface Playbook {
    name: string;
    items: {
        key: string;
        item: string;
        playbook: string;
        description?: string;
    }[];
    defaults: Policy;
    presets: {
        id: string;
        name: string;
        policy: Policy;
    }[];
}
export interface User {
    email: string;
    name: string;
    firm_name: string;
    firm_code: string;
    initials: string;
    role: 'admin' | 'reviewer' | 'viewer';
    firm_id: string;
}
export interface Capabilities {
    llm: { available: boolean };
    ocr: { available: boolean };
    translation: { available: boolean };
    dictation: { available: boolean };
}
export interface Extraction {
    has_original?: boolean;
    id: string; name: string; text: string;
    pages: { number: number; text: string; start: number; end: number; confidence?: number }[];
    warnings: string[]; source_sha256?: string; method: string; status: string;
}
export interface Health {
    status: string;
    dbx: {
        ok: boolean;
        detail?: string;
    };
}
export interface ExportRecord {
    id: string;
    client_id?: string;
    client_name: string;
    matter?: string;
    kind: string;
    created_at: number;
    body?: string;
}
function serverMessage(value: unknown): string {
    if (typeof value === 'string')
        return value;
    if (Array.isArray(value))
        return value.map(v => typeof v?.msg === 'string' ? v.msg : 'Invalid field').join('; ');
    return 'The server could not complete this request.';
}
export async function request(path: string, init: RequestInit = {}, authenticated = true): Promise<Response> {
    const headers = new Headers(init.headers);
    if (init.body && !(init.body instanceof FormData))
        headers.set('Content-Type', 'application/json');
    if (!['GET', 'HEAD', 'OPTIONS'].includes((init.method || 'GET').toUpperCase())) {
        const cookie = document.cookie.split('; ').find(value => value.startsWith('jurix_csrf='));
        if (cookie) headers.set('X-CSRF-Token', decodeURIComponent(cookie.slice('jurix_csrf='.length)));
    }
    const token = localStorage.getItem('jurix_token');
    if (token && authenticated) headers.set('Authorization', `Bearer ${token}`);
    let response: Response;
    try {
        response = await fetch(path, { ...init, headers, credentials: 'same-origin' });
    }
    catch {
        throw new Error('Unable to reach Jurix. Check the connection and try again.');
    }
    if (response.status === 401 && authenticated) {
        localStorage.removeItem('jurix_token');
        window.dispatchEvent(new Event('jurix-session-expired'));
    }
    if (!response.ok) {
        let error = `Request failed (${response.status}).`;
        try {
            const data = await response.json();
            error = serverMessage(data.detail || data.error || error);
        }
        catch { /* keep status */ }
        throw new Error(error);
    }
    return response;
}
export async function api<T>(path: string, init: RequestInit = {}, authenticated = true): Promise<T> { return (await request(path, init, authenticated)).json() as Promise<T>; }
export const json = (value: unknown) => JSON.stringify(value);
export const message = (error: unknown) => error instanceof Error ? error.message : 'Something went wrong. Try again.';
export const dateTime = (value?: number) => value ? new Date(value * 1000).toLocaleString() : 'Not run yet';
export const findingsFor = (matter: Matter): Finding[] => matter.document_runs?.length ? matter.document_runs.flatMap(run => run.items) : (matter.items || []);
export const reviewLabel = (status: string) => (({ open: 'Open', in_review: 'In review', resolved: 'Resolved', accepted_risk: 'Accepted risk' } as Record<string, string>)[status] || status);
export const severityLabel = (state: string) => (({ high: 'High risk', medium: 'Medium risk', missing: 'Missing', clear: 'Meets check' } as Record<string, string>)[state] || state);
export async function downloadExport(id: string) {
    await downloadFile(`/api/exports/${encodeURIComponent(id)}`, {}, `jurix-review-${id}.md`);
}
export async function downloadFile(path: string, init: RequestInit = {}, fallback = 'jurix-document') {
    const response = await request(path, init);
    const url = URL.createObjectURL(await response.blob());
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = response.headers.get('Content-Disposition')?.match(/filename="([^"]+)"/)?.[1] || fallback;
    anchor.style.display = 'none';
    document.body.appendChild(anchor);
    anchor.click();
    setTimeout(() => {
        anchor.remove();
        URL.revokeObjectURL(url);
    }, 60000);
}
