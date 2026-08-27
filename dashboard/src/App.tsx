import type { ReactNode } from 'react';
import { BrowserRouter as Router, Routes, Route, Navigate } from 'react-router-dom';
import ProjectsPage from './pages/ProjectsPage';
import ClusterDashboard from './pages/ClusterDashboard';
import LoginPage from './pages/LoginPage';
import Sidebar from './components/Sidebar';
import SettingsPage from './pages/SettingsPage';
import Header from './components/Header';
import { ThemeProvider } from './components/ThemeProvider';
import { ToastProvider } from './components/Toaster';
import { TenantProvider } from './components/TenantProvider';
import CommandPalette from './components/CommandPalette';
import './index.css';

const ProtectedRoute = ({ children }: { children: ReactNode }) => {
  const token = localStorage.getItem('dbx_token');
  if (!token) {
    return <Navigate to="/login" replace />;
  }
  return children;
};

function GlobalLayout({ children }: { children: ReactNode }) {
  return (
    <div className="flex h-full w-full bg-[var(--bg-primary)]">
      <Sidebar clusterId="" />
      <main className="flex-1 flex flex-col overflow-hidden min-w-0">
        <Header />
        <div className="flex-1 overflow-y-auto">
          {children}
        </div>
      </main>
    </div>
  );
}

export default function App() {
  return (
    <ThemeProvider>
      <ToastProvider>
        <Router>
          <TenantProvider>
            <div className="app-root">
              <Routes>
                <Route path="/login" element={<LoginPage />} />

                <Route path="/" element={<ProtectedRoute><ProjectsPage /></ProtectedRoute>} />

                <Route
                  path="/settings/*"
                  element={
                    <ProtectedRoute>
                      <GlobalLayout>
                        <SettingsPage />
                      </GlobalLayout>
                    </ProtectedRoute>
                  }
                />

                <Route
                  path="/cluster/:id/*"
                  element={<ProtectedRoute><ClusterDashboard /></ProtectedRoute>}
                />

                <Route path="*" element={<Navigate to="/" replace />} />
              </Routes>
            </div>
            <CommandPalette />
          </TenantProvider>
        </Router>
      </ToastProvider>
    </ThemeProvider>
  );
}
