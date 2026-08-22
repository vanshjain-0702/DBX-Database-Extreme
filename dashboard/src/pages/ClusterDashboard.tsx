
import { Routes, Route, Navigate, useParams } from 'react-router-dom';
import Sidebar from '../components/Sidebar';
import Header from '../components/Header';
import OverviewPage from './OverviewPage';
import DataExplorerPage from './DataExplorerPage';
import VectorPlaygroundPage from './VectorPlaygroundPage';
import ConsolePage from './ConsolePage';
import HardwarePage from './HardwarePage';
import StoragePage from './StoragePage';
import NetworkPage from './NetworkPage';
import HostingPerformancePage from './HostingPerformancePage';

export default function ClusterDashboard() {
  const { id } = useParams();

  return (
    <div className="cluster-dashboard h-full flex w-full">
      <Sidebar clusterId={id || ''} />

      <main className="main-content flex-1 flex flex-col min-w-0 overflow-hidden bg-[#f8fafc]">
        <Header />
        <div className="flex-1 overflow-y-auto">
          <Routes>
            <Route path="overview" element={<OverviewPage clusterId={id || ''} />} />
            <Route path="explorer" element={<DataExplorerPage clusterId={id || ''} />} />
            <Route path="hosting" element={<HostingPerformancePage clusterId={id || ''} />} />
            <Route path="vector" element={<VectorPlaygroundPage clusterId={id || ''} />} />
            <Route path="terminal" element={<ConsolePage clusterId={id || ''} />} />
            <Route path="hardware" element={<HardwarePage clusterId={id || ''} />} />
            <Route path="storage" element={<StoragePage clusterId={id || ''} />} />
            <Route path="network" element={<NetworkPage clusterId={id || ''} />} />
            <Route path="*" element={<Navigate to={`/cluster/${id}/overview`} replace />} />
          </Routes>
        </div>
      </main>
    </div>
  );
}
