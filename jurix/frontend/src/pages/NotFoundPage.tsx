import { Link } from 'react-router-dom';

export default function NotFoundPage() {
    return <section className="surface"><div className="empty-state">
        <p className="eyebrow">PAGE NOT FOUND</p><h1>This page isn't in the docket.</h1>
        <p>The address may have changed. Return to your matters to continue reviewing.</p>
        <Link to="/matters" className="button primary">Open the matter docket</Link>
    </div></section>;
}
