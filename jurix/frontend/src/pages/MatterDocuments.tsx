import { useOutletContext } from 'react-router-dom';
import { FileText, Upload } from 'lucide-react';

export default function MatterDocuments() {
  const { matter } = useOutletContext<{matter: any}>();
  
  return (
    <div className="bg-black/40 backdrop-blur-xl rounded-2xl border border-[#C4A574]/20 shadow-[0_10px_30px_rgba(0,0,0,0.5)] p-6 animate-in fade-in slide-in-from-bottom-4 duration-500 max-w-4xl">
      <div className="flex items-center justify-between border-b border-[#C4A574]/20 pb-4 mb-6">
        <h3 className="font-serif text-xl font-bold flex items-center gap-2 text-white">
          <FileText className="text-[#C4A574]" /> Document Corpus
        </h3>
        <button className="bg-white/10 hover:bg-white/20 text-white px-4 py-2 rounded-xl text-sm font-medium flex items-center gap-2 transition-colors border border-white/10">
          <Upload size={16} /> Upload Contract
        </button>
      </div>
      
      {matter.documents?.length > 0 ? (
        <ul className="space-y-3">
          {matter.documents.map((doc: any, i: number) => (
            <li key={i} className="flex items-center justify-between p-4 bg-black/50 rounded-xl border border-gray-700 hover:border-[#C4A574]/40 transition-colors group">
              <div className="flex items-center gap-4">
                <div className="p-2 bg-[#C4A574]/10 text-[#C4A574] rounded-lg"><FileText size={20} /></div>
                <div>
                  <span className="block font-medium text-gray-200 group-hover:text-white transition-colors">{doc.name}</span>
                  <span className="text-xs text-gray-500">Ingested on {new Date(doc.ingested_at * 1000).toLocaleDateString()}</span>
                </div>
              </div>
              <span className="text-xs uppercase tracking-wider text-[#C4A574] bg-[#C4A574]/10 px-3 py-1.5 rounded-lg border border-[#C4A574]/20">{doc.chunk_count} chunks vectorised</span>
            </li>
          ))}
        </ul>
      ) : (
        <div className="text-sm text-gray-500 flex flex-col items-center justify-center py-12 bg-black/30 rounded-xl border border-dashed border-gray-700">
          <Upload className="mb-3 text-gray-600" size={32} />
          <p>No documents ingested. Upload a contract to begin.</p>
        </div>
      )}
    </div>
  );
}
