import { Loader2, AlertCircle } from 'lucide-react';
import type { ReactNode } from 'react';
export function Loading({ text = 'Loading workspace…' }: {
    text?: string;
}) { return <div className="loading-state" role="status"><Loader2 className="spin" size={20}/>{text}</div>; }
export function ErrorNotice({ error, retry }: {
    error: string;
    retry?: () => void;
}) { if (!error)
    return null; return <div className="notice error" role="alert"><AlertCircle size={18}/><span>{error}</span>{retry && <button className="button subtle" onClick={retry}>Try again</button>}</div>; }
export function EmptyState({ title, text, children }: {
    title: string;
    text: string;
    children?: ReactNode;
}) { return <div className="empty-state"><h3>{title}</h3><p>{text}</p>{children}</div>; }
