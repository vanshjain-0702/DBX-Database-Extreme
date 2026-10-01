import { useState, useRef, useEffect } from 'react';
import { useOutletContext } from 'react-router-dom';
import { MessageSquare, Send } from 'lucide-react';

export default function MatterCounsel() {
  const { matter } = useOutletContext<{matter: any}>();
  
  const [chatLog, setChatLog] = useState<{role: 'user'|'system', text: string}[]>([]);
  const [chatInput, setChatInput] = useState('');
  const chatBottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (chatBottomRef.current) {
      chatBottomRef.current.scrollIntoView({ behavior: 'smooth' });
    }
  }, [chatLog]);

  const [isSearching, setIsSearching] = useState(false);

  const handleAskCounsel = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!chatInput.trim() || isSearching) return;
    
    setChatLog(prev => [...prev, { role: 'user', text: chatInput }]);
    const query = chatInput;
    setChatInput('');
    setIsSearching(true);
    
    try {
      const token = localStorage.getItem('jurix_token');
      const res = await fetch(`/api/matters/${matter?.id}/search`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`
        },
        body: JSON.stringify({ query })
      });

      if (!res.ok) throw new Error('Search failed');
      const data = await res.json();
      
      let answerText = `I searched the DBX tenant and found ${data.hits?.length || 0} relevant passages for "${query}".\n\n`;
      if (data.hits && data.hits.length > 0) {
        data.hits.forEach((hit: any, idx: number) => {
          answerText += `[${idx+1}] "...${hit.text}..." (Source: ${hit.doc_name})\n\n`;
        });
      }

      setChatLog(prev => [...prev, { role: 'system', text: answerText }]);
    } catch (err) {
      setChatLog(prev => [...prev, { role: 'system', text: `Error connecting to DBX tenant. Please try again.` }]);
    } finally {
      setIsSearching(false);
    }
  };

  const handlePromptClick = (prompt: string) => {
    setChatInput(prompt);
  };

  return (
    <div className="bg-black/40 backdrop-blur-xl rounded-2xl border border-[#C4A574]/20 shadow-[0_10px_30px_rgba(0,0,0,0.5)] overflow-hidden h-full flex flex-col animate-in fade-in slide-in-from-bottom-4 duration-500">
      <div className="p-6 border-b border-[#C4A574]/20 bg-black/60 flex items-center justify-between shrink-0">
        <div className="flex items-center gap-3">
          <div className="p-2 bg-[#C4A574]/10 text-[#C4A574] rounded-lg"><MessageSquare size={20} /></div>
          <div>
            <h2 className="text-xl font-serif text-white">Counsel AI</h2>
            <p className="text-xs text-gray-400">Secure chat with {matter?.tenant_id} vectors only.</p>
          </div>
        </div>
      </div>
      
      <div className="flex-1 overflow-y-auto p-6 space-y-4 custom-scrollbar">
        {chatLog.length === 0 && (
          <div className="h-full flex flex-col items-center justify-center text-center space-y-4 opacity-50">
            <MessageSquare size={48} className="text-[#C4A574]" />
            <p className="text-gray-300 max-w-sm">Ask any question about the ingested documents. Counsel will exclusively query this matter's sealed DBX tenant.</p>
          </div>
        )}
        {chatLog.map((msg, i) => (
          <div key={i} className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}>
            <div className={`p-4 rounded-2xl max-w-[70%] text-sm shadow-md ${msg.role === 'user' ? 'bg-[#C4A574] text-[#0a0806] font-medium rounded-tr-sm' : 'bg-[#1a1614] text-gray-200 rounded-tl-sm border border-[#C4A574]/20'}`}>
              {msg.text}
            </div>
          </div>
        ))}
        <div ref={chatBottomRef} />
      </div>

      <div className="p-6 bg-black/60 border-t border-[#C4A574]/20 shrink-0">
        <div className="flex flex-wrap gap-2 mb-4 justify-center">
            {['Indemnity cap?', 'Governing law?', 'Renewal notice?'].map(prompt => (
              <button 
                key={prompt}
                onClick={() => handlePromptClick(prompt)}
                className="text-[10px] uppercase tracking-wider bg-white/5 hover:bg-white/10 text-gray-300 px-3 py-1.5 rounded-full border border-white/10 transition-colors"
              >
                {prompt}
              </button>
            ))}
        </div>
        <form onSubmit={handleAskCounsel} className="relative max-w-4xl mx-auto">
          <input 
            type="text" 
            value={chatInput}
            onChange={e => setChatInput(e.target.value)}
            placeholder="Ask Counsel a legal question..."
            className="w-full bg-[#0a0806] border border-[#C4A574]/30 rounded-xl pl-5 pr-14 py-4 focus:outline-none focus:ring-1 focus:ring-[#C4A574] text-white text-sm shadow-inner"
          />
          <button 
            type="submit"
            disabled={!chatInput.trim()}
            className="absolute right-3 top-3 p-2 bg-[#C4A574] text-black rounded-lg disabled:opacity-50 hover:bg-[#d8b884] transition-all shadow-[0_0_10px_rgba(196,165,116,0.3)]"
          >
            <Send size={18} />
          </button>
        </form>
      </div>
    </div>
  );
}
