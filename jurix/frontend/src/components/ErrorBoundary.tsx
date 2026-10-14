import { Component, type ReactNode } from 'react';

export default class ErrorBoundary extends Component<{children: ReactNode}, {failed: boolean}> {
    state = {failed: false};
    static getDerivedStateFromError() { return {failed: true}; }
    render() {
        if (this.state.failed) return <main className="surface error-fallback" role="alert">
            <p className="eyebrow">WORKSPACE INTERRUPTED</p><h1>We couldn't display this page.</h1>
            <p>Reload the workspace to recover. If the issue continues, share the page and action with your administrator.</p>
            <button className="button primary" onClick={() => window.location.reload()}>Reload workspace</button>
        </main>;
        return this.props.children;
    }
}
