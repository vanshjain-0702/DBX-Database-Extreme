import { HashRouter as Router, Routes, Route, Navigate } from 'react-router-dom';
import { ReactNode } from 'react';

import LoginPage from './pages/LoginPage';
import MattersPage from './pages/MattersPage';
import MatterLayout from './pages/MatterLayout';
import MatterDashboard from './pages/MatterDashboard';
import MatterFindings from './pages/MatterFindings';
import MatterDocuments from './pages/MatterDocuments';
import MatterCounsel from './pages/MatterCounsel';
import GlobalLayout from './layouts/GlobalLayout';

const ProtectedRoute = ({ children }: { children: ReactNode }) => {
  const token = localStorage.getItem('jurix_token');
  if (!token) return <Navigate to="/login" replace />;
  return children;
};

export default function App() {
  return (
    <Router>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/*" element={
          <ProtectedRoute>
            <GlobalLayout>
              <Routes>
                <Route path="/" element={<Navigate to="/matters" replace />} />
                <Route path="/matters" element={<MattersPage />} />
                <Route path="/matters/:id" element={<MatterLayout />}>
                  <Route index element={<Navigate to="dashboard" replace />} />
                  <Route path="dashboard" element={<MatterDashboard />} />
                  <Route path="findings" element={<MatterFindings />} />
                  <Route path="documents" element={<MatterDocuments />} />
                  <Route path="counsel" element={<MatterCounsel />} />
                </Route>
                <Route path="/checklists" element={<div className="text-4xl font-serif">Checklists</div>} />
                <Route path="/exports" element={<div className="text-4xl font-serif">Exports</div>} />
                <Route path="/firm" element={<div className="text-4xl font-serif">Firm Settings</div>} />
              </Routes>
            </GlobalLayout>
          </ProtectedRoute>
        } />
      </Routes>
    </Router>
  );
}
