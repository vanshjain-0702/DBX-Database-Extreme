import { HashRouter as Router, Routes, Route, Navigate } from 'react-router-dom';
import { type ReactNode } from 'react';
import { SessionProvider, useSession } from './lib/session';
import { Loading, ErrorNotice } from './components/Feedback';
import RegulationsPage from './pages/RegulationsPage';
import LoginPage from './pages/LoginPage';
import MattersPage from './pages/MattersPage';
import MatterLayout from './pages/MatterLayout';
import MatterDashboard from './pages/MatterDashboard';
import MatterFindings from './pages/MatterFindings';
import MatterDocuments from './pages/MatterDocuments';
import MatterCounsel from './pages/MatterCounsel';
import MatterBilling from './pages/MatterBilling';
import ChecklistsPage from './pages/ChecklistsPage';
import ExportsPage from './pages/ExportsPage';
import FirmPage from './pages/FirmPage';
import GlobalLayout from './layouts/GlobalLayout';
import NotFoundPage from './pages/NotFoundPage';
function ProtectedRoute({ children }: {
    children: ReactNode;
}) {
    const { user, loading, error, reload } = useSession();
    if (loading) return <Loading text="Checking your session…"/>;
    if (error) return <div className="session-error"><ErrorNotice error={error} retry={() => void reload()}/></div>;
    return user ? children : <Navigate to="/login" replace/>;
}
export default function App() {
    return <SessionProvider><Router><Routes>
    <Route path="/login" element={<LoginPage />}/>
    <Route path="/*" element={<ProtectedRoute><GlobalLayout><Routes>
      <Route path="/" element={<Navigate to="/matters" replace/>}/>
      <Route path="/matters" element={<MattersPage />}/>
      <Route path="/matters/:id" element={<MatterLayout />}>
        <Route index element={<Navigate to="dashboard" replace/>}/>
        <Route path="dashboard" element={<MatterDashboard />}/>
        <Route path="findings" element={<MatterFindings />}/>
        <Route path="documents" element={<MatterDocuments />}/>
        <Route path="counsel" element={<MatterCounsel />}/>
        <Route path="billing" element={<MatterBilling />}/>
        <Route path="*" element={<NotFoundPage />}/>
      </Route>
      <Route path="/checklists" element={<ChecklistsPage />}/>
      <Route path="/exports" element={<ExportsPage />}/>
      <Route path="/firm" element={<FirmPage />}/>
      <Route path="/regulations" element={<RegulationsPage />}/>
      <Route path="*" element={<NotFoundPage />}/>
    </Routes></GlobalLayout></ProtectedRoute>}/>
  </Routes></Router></SessionProvider>;
}
