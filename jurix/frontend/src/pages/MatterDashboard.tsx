import { useOutletContext } from 'react-router-dom';
import { ShieldAlert, FileText, CheckCircle2 } from 'lucide-react';

export default function MatterDashboard() {
  const { matter } = useOutletContext<{matter: any}>();
  
  return (
    <div className="grid grid-cols-1 md:grid-cols-3 gap-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
      <div className="bg-black/40 backdrop-blur-xl rounded-2xl border border-[#C4A574]/20 p-6 shadow-lg">
        <div className="text-[#C4A574] mb-4"><ShieldAlert size={32} /></div>
        <h3 className="text-3xl font-serif text-white mb-1">{matter.high || 0}</h3>
        <p className="text-gray-400 text-sm uppercase tracking-wider font-medium">High Risk Items</p>
      </div>
      <div className="bg-black/40 backdrop-blur-xl rounded-2xl border border-[#C4A574]/20 p-6 shadow-lg">
        <div className="text-[#C4A574] mb-4"><FileText size={32} /></div>
        <h3 className="text-3xl font-serif text-white mb-1">{matter.documents?.length || 0}</h3>
        <p className="text-gray-400 text-sm uppercase tracking-wider font-medium">Documents Ingested</p>
      </div>
      <div className="bg-black/40 backdrop-blur-xl rounded-2xl border border-[#C4A574]/20 p-6 shadow-lg">
        <div className="text-[#C4A574] mb-4"><CheckCircle2 size={32} /></div>
        <h3 className="text-3xl font-serif text-white mb-1">{matter.status === 'cleared' ? 'Cleared' : 'In Review'}</h3>
        <p className="text-gray-400 text-sm uppercase tracking-wider font-medium">Audit Status</p>
      </div>
    </div>
  );
}
