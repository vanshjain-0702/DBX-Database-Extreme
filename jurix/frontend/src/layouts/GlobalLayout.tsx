import { ReactNode } from 'react';
import Sidebar from '../components/Sidebar';
import Header from '../components/Header';
import MotionBackground from '../components/MotionBackground';

export default function GlobalLayout({ children }: { children: ReactNode }) {
  return (
    <div className="flex h-screen w-full relative overflow-hidden bg-[#0a0806] text-white">
      {/* Background layer */}
      <MotionBackground />

      {/* App Shell over background */}
      <div className="relative z-10 flex h-full w-full bg-black/40">
        <Sidebar />
        <main className="flex-1 flex flex-col min-w-0 h-full backdrop-blur-[2px]">
          <Header />
          
          <div className="flex-1 overflow-y-auto p-8 custom-scrollbar">
            <div className="max-w-7xl mx-auto h-full">
              {children}
            </div>
          </div>
        </main>
      </div>

      <style>{`
        .custom-scrollbar::-webkit-scrollbar {
          width: 8px;
        }
        .custom-scrollbar::-webkit-scrollbar-track {
          background: rgba(255, 255, 255, 0.02);
        }
        .custom-scrollbar::-webkit-scrollbar-thumb {
          background: rgba(196, 165, 116, 0.3);
          border-radius: 4px;
        }
        .custom-scrollbar::-webkit-scrollbar-thumb:hover {
          background: rgba(196, 165, 116, 0.5);
        }
      `}</style>
    </div>
  );
}
