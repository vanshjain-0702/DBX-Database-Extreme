import { useOutletContext } from 'react-router-dom';
import { ShieldAlert, FileText, PlayCircle } from 'lucide-react';

export default function MatterFindings() {
  const { matter } = useOutletContext<{matter: any}>();
  
  return (
    <div className="bg-black/40 backdrop-blur-xl rounded-2xl border border-[#C4A574]/20 shadow-[0_10px_30px_rgba(0,0,0,0.5)] overflow-hidden animate-in fade-in slide-in-from-bottom-4 duration-500 h-full flex flex-col">
      <div className="bg-black/60 px-6 py-5 border-b border-[#C4A574]/20 flex items-center justify-between shrink-0">
        <div className="flex items-center gap-3">
          <span className="font-serif text-xl text-white">Checklist Findings</span>
          <span className="bg-[#C4A574]/20 text-[#C4A574] px-3 py-1 rounded-full text-sm font-sans font-medium border border-[#C4A574]/30">
            {matter.items?.length || 0}
          </span>
        </div>
        <button className="bg-[#C4A574] hover:bg-[#d8b884] text-[#0a0806] px-4 py-2 rounded-xl font-bold flex items-center gap-2 shadow-[0_0_15px_rgba(196,165,116,0.3)] transition-all">
          <PlayCircle size={16} /> Run Audit
        </button>
      </div>
      <div className="divide-y divide-[#C4A574]/10 overflow-y-auto custom-scrollbar flex-1">
        {matter.items?.length > 0 ? (
          matter.items.map((item: any, i: number) => (
            <div key={i} className="p-6 hover:bg-white/5 transition-colors group">
              <div className="flex items-start gap-5">
                <div className={`p-3 rounded-xl shrink-0 border ${item.state === 'high' ? 'bg-red-900/30 text-red-400 border-red-500/30' : item.state === 'missing' ? 'bg-orange-900/30 text-orange-400 border-orange-500/30' : 'bg-green-900/30 text-green-400 border-green-500/30'}`}>
                  <ShieldAlert size={24} />
                </div>
                <div className="flex-1">
                  <h4 className="font-serif font-bold text-xl text-white mb-1 group-hover:text-[#C4A574] transition-colors">{item.item}</h4>
                  <p className="text-sm font-medium text-gray-400 mb-4">Playbook: <span className="text-gray-300">{item.playbook}</span></p>
                  <p className="text-sm text-gray-300 bg-black/50 p-4 rounded-xl border border-gray-700 mb-4 leading-relaxed italic">
                    "{item.quote}"
                  </p>
                  <div className="text-xs text-[#C4A574] font-mono flex items-center gap-2 bg-[#C4A574]/10 w-fit px-3 py-1.5 rounded-lg border border-[#C4A574]/20">
                    <FileText size={14} /> {item.doc_name} &middot; {item.citation}
                  </div>
                </div>
              </div>
            </div>
          ))
        ) : (
          <div className="p-12 text-center text-gray-400 text-sm">
            No findings yet. Run the checklist to analyze the documents.
          </div>
        )}
      </div>
    </div>
  );
}
