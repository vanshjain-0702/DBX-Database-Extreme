import type { ReactNode } from 'react';
import { BrowserRouter as Router, Routes, Route, Navigate } from 'react-router-dom';
import ProjectsPage from './pages/ProjectsPage';
import ClusterDashboard from './pages/ClusterDashboard';
import LoginPage from './pages/LoginPage';
import Sidebar from './components/Sidebar';
import SettingsPage from './pages/SettingsPage';
import './index.css';

const ProtectedRoute = ({ children }: { children: ReactNode }) => {
  const token = localStorage.getItem('dbx_token');
  if (!token) {
    return <Navigate to="/login" replace />;
  }
  return children;
};

import Header from './components/Header';

/**
 * GlobalLayout wraps pages that need the full sidebar+content layout
 * but are not inside a specific cluster (e.g. /settings).
 */
function GlobalLayout({ children }: { children: ReactNode }) {
  return (
    <div className="flex h-full w-full bg-[#f8fafc]">
      {/* Sidebar with empty clusterId for global pages */}
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

import { ThemeProvider } from './components/ThemeProvider';
import { ToastProvider } from './components/Toaster';
import CommandPalette from './components/CommandPalette';

export default function App() {
  return (
    <ThemeProvider>
      <ToastProvider>
        <Router>
          <div className="app-root text-slate-900 dark:text-slate-100 bg-white dark:bg-slate-950 transition-colors duration-200">
            <Routes>
              <Route path="/login" element={<LoginPage />} />

              {/* Landing / Org View — has its own internal layout (no sidebar) */}
              <Route path="/" element={<ProtectedRoute><ProjectsPage /></ProtectedRoute>} />

              {/* Global Settings — uses GlobalLayout with sidebar */}
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

              {/* Specific Cluster Dashboard */}
              <Route
                path="/cluster/:id/*"
                element={<ProtectedRoute><ClusterDashboard /></ProtectedRoute>}
              />

              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
          </div>
          <CommandPalette />
        </Router>
      </ToastProvider>
    </ThemeProvider>
  );
}
