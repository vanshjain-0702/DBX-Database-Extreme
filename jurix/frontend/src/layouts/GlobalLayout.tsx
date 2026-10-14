import { type ReactNode, useCallback, useEffect, useRef, useState } from 'react';
import Sidebar from '../components/Sidebar';
import Header from '../components/Header';
import { useLocation } from 'react-router-dom';

export default function GlobalLayout({ children }: { children: ReactNode }) {
    const [navigationOpen, setNavigationOpen] = useState(false);
    const [mobile, setMobile] = useState(() => window.matchMedia('(max-width: 760px)').matches);
    const navigationButton = useRef<HTMLButtonElement>(null);
    const drawer = useRef<HTMLElement>(null);
    const main = useRef<HTMLElement>(null);
    const location = useLocation();
    // Derive a stable page key from the route for CSS background images
    const pageKey = (() => {
        const parts = location.pathname.split('/').filter(Boolean);
        if (!parts.length || parts[0] === 'matters' && parts.length === 1) return 'matters';
        if (parts[0] === 'matters' && parts.length >= 3) return parts[2]; // dashboard | findings | documents | counsel | billing
        return parts[0]; // checklists | exports | firm | regulations
    })();
    useEffect(() => {
        const parts = location.pathname.split('/').filter(Boolean);
        const label = parts[parts.length - 1] || 'Matters';
        document.title = `${label.charAt(0).toUpperCase() + label.slice(1)} · JURIX`;
        const frame = requestAnimationFrame(() => { main.current?.focus({preventScroll: true}); main.current?.scrollTo({top: 0}); });
        return () => cancelAnimationFrame(frame);
    }, [location.pathname]);

    const closeNavigation = useCallback(() => {
        setNavigationOpen(false);
        requestAnimationFrame(() => navigationButton.current?.focus({ preventScroll: true }));
    }, []);

    useEffect(() => {
        const query = window.matchMedia('(max-width: 760px)');
        const update = () => {
            setMobile(query.matches);
            if (!query.matches) setNavigationOpen(false);
        };
        query.addEventListener('change', update);
        return () => query.removeEventListener('change', update);
    }, []);

    useEffect(() => {
        if (!mobile || !navigationOpen) return;
        const elements = () => Array.from(drawer.current?.querySelectorAll<HTMLElement>('a[href], button:not([disabled]), [tabindex="0"]') || []);
        const frame = requestAnimationFrame(() => elements()[0]?.focus({ preventScroll: true }));
        const handleKey = (event: KeyboardEvent) => {
            if (event.key === 'Escape') {
                event.preventDefault();
                closeNavigation();
            } else if (event.key === 'Tab') {
                const targets = elements();
                const first = targets[0];
                const last = targets[targets.length - 1];
                if (event.shiftKey && document.activeElement === first) {
                    event.preventDefault();
                    last?.focus();
                } else if (!event.shiftKey && document.activeElement === last) {
                    event.preventDefault();
                    first?.focus();
                }
            }
        };
        document.addEventListener('keydown', handleKey);
        return () => {
            cancelAnimationFrame(frame);
            document.removeEventListener('keydown', handleKey);
        };
    }, [mobile, navigationOpen, closeNavigation]);

    return (
        <div className="app-shell">
            <a className="skip-link" href="#main-content" onClick={event => {
                event.preventDefault();
                main.current?.focus({ preventScroll: true });
            }}>Skip to content</a>
            {mobile && navigationOpen && (
                <button className="navigation-scrim" aria-label="Close navigation" tabIndex={-1} onClick={closeNavigation} />
            )}
            <Sidebar open={navigationOpen} close={closeNavigation} hidden={mobile && !navigationOpen} drawerRef={drawer} mobile={mobile} />
            <div className="app-main" inert={mobile && navigationOpen}>
                <Header openNavigation={() => setNavigationOpen(current => !current)} navigationOpen={navigationOpen} navigationButtonRef={navigationButton} />
                <main id="main-content" ref={main} tabIndex={-1} className="page-scroll" data-page={pageKey}>
                    <div className="page-content">{children}</div>
                </main>
            </div>
        </div>
    );
}
