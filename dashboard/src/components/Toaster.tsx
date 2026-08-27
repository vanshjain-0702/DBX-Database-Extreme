import { createContext, useContext, useState, useCallback } from 'react';
import type { ReactNode } from 'react';
import { CheckCircle2, XCircle, Info, X } from 'lucide-react';

type ToastType = 'success' | 'error' | 'info';

type Toast = {
  id: string;
  message: string;
  type: ToastType;
};

type ToastContextType = {
  toast: {
    success: (msg: string) => void;
    error: (msg: string) => void;
    info: (msg: string) => void;
  };
};

const ToastContext = createContext<ToastContextType | undefined>(undefined);

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const removeToast = useCallback((id: string) => {
    setToasts(prev => prev.filter(t => t.id !== id));
  }, []);

  const addToast = useCallback((message: string, type: ToastType) => {
    const id = Math.random().toString(36).substring(2, 9);
    setToasts(prev => [...prev, { id, message, type }]);
    setTimeout(() => removeToast(id), 5000);
  }, [removeToast]);

  const toast = {
    success: (msg: string) => addToast(msg, 'success'),
    error: (msg: string) => addToast(msg, 'error'),
    info: (msg: string) => addToast(msg, 'info'),
  };

  return (
    <ToastContext.Provider value={{ toast }}>
      {children}
      <div className="fixed bottom-5 right-5 z-50 flex flex-col gap-2 max-w-sm w-full pointer-events-none">
        {toasts.map(t => (
          <div
            key={t.id}
            className="pointer-events-auto flex items-center justify-between gap-3 px-3.5 py-2.5 rounded-md border bg-[var(--bg-panel)] border-[var(--border-color)] text-[var(--text-primary)]"
          >
            <div className="flex items-center gap-2.5">
              {t.type === 'success' && <CheckCircle2 size={16} className="text-[var(--success)] shrink-0" />}
              {t.type === 'error' && <XCircle size={16} className="text-[var(--error)] shrink-0" />}
              {t.type === 'info' && <Info size={16} className="text-[var(--text-muted)] shrink-0" />}
              <span className="text-[13px] font-medium">{t.message}</span>
            </div>
            <button type="button" onClick={() => removeToast(t.id)} className="text-[var(--text-muted)] hover:text-[var(--text-primary)]">
              <X size={14} />
            </button>
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

/* eslint-disable-next-line react-refresh/only-export-components */
export const useToast = () => {
  const context = useContext(ToastContext);
  if (!context) throw new Error('useToast must be used within ToastProvider');
  return context.toast;
};
