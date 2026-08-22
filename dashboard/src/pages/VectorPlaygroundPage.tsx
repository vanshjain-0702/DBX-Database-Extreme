import { useState } from 'react';
import { Network, Play, TerminalSquare, AlertTriangle, Loader2 } from 'lucide-react';
import { fetchWithAuth } from '../api';
import { pipeline, env } from '@xenova/transformers';
import * as THREE from 'three';
import gsap from 'gsap';
import { useRef, useEffect } from 'react';

function VectorGalaxy({ results }: { results: any[] }) {
  const mountRef = useRef<HTMLDivElement>(null);
  
  useEffect(() => {
    if (!mountRef.current) return;
    const scene = new THREE.Scene();
    scene.fog = new THREE.FogExp2(0x0f172a, 0.02);

    const w = mountRef.current.clientWidth;
    const h = mountRef.current.clientHeight;
    const camera = new THREE.PerspectiveCamera(75, w / h, 0.1, 1000);
    
    const renderer = new THREE.WebGLRenderer({ alpha: true, antialias: true });
    renderer.setSize(w, h);
    renderer.setPixelRatio(window.devicePixelRatio);
    mountRef.current.appendChild(renderer.domElement);

    // Particles
    const geometry = new THREE.BufferGeometry();
    const count = 3000;
    const positions = new Float32Array(count * 3);
    for(let i=0; i<count*3; i++) {
       positions[i] = (Math.random() - 0.5) * 60;
    }
    geometry.setAttribute('position', new THREE.BufferAttribute(positions, 3));
    
    // Result nodes
    const resultGeo = new THREE.BufferGeometry();
    const resultPos = new Float32Array(results.length * 3);
    results.forEach((_, i) => {
       resultPos[i*3] = (Math.random() - 0.5) * 15;
       resultPos[i*3+1] = (Math.random() - 0.5) * 15;
       resultPos[i*3+2] = (Math.random() - 0.5) * 15;
    });
    resultGeo.setAttribute('position', new THREE.BufferAttribute(resultPos, 3));

    const material = new THREE.PointsMaterial({ size: 0.1, color: 0x64748b, transparent: true, opacity: 0.3 });
    const resultMat = new THREE.PointsMaterial({ size: 0.6, color: 0xef4444, transparent: true, opacity: 1.0 });

    const particles = new THREE.Points(geometry, material);
    const resultParticles = new THREE.Points(resultGeo, resultMat);
    scene.add(particles);
    scene.add(resultParticles);

    camera.position.z = 25;

    let reqId: number;
    const animate = () => {
      reqId = requestAnimationFrame(animate);
      particles.rotation.y += 0.0005;
      particles.rotation.x += 0.0002;
      resultParticles.rotation.y += 0.002;
      resultParticles.rotation.x += 0.001;
      renderer.render(scene, camera);
    };
    animate();

    if (results.length > 0) {
      gsap.fromTo(camera.position, 
        { z: 40, y: 10 },
        { z: 12, y: 0, duration: 2.5, ease: "power3.out" }
      );
    }

    const handleResize = () => {
      if(!mountRef.current) return;
      camera.aspect = mountRef.current.clientWidth / mountRef.current.clientHeight;
      camera.updateProjectionMatrix();
      renderer.setSize(mountRef.current.clientWidth, mountRef.current.clientHeight);
    }
    window.addEventListener('resize', handleResize);

    return () => {
      window.removeEventListener('resize', handleResize);
      cancelAnimationFrame(reqId);
      if (mountRef.current && renderer.domElement.parentNode === mountRef.current) {
        mountRef.current.removeChild(renderer.domElement);
      }
      geometry.dispose();
      material.dispose();
      resultGeo.dispose();
      resultMat.dispose();
      renderer.dispose();
    };
  }, [results]);

  return <div ref={mountRef} className="absolute inset-0 z-0 opacity-40 pointer-events-none" />;
}

// Disable local models to prevent SPA catching /models requests and returning index.html (which causes JSON parse errors)
env.allowLocalModels = false;

// Global cache for the pipeline
let extractorPipe: any = null;

export default function VectorPlaygroundPage({ clusterId }: { clusterId: string }) {
  const [query, setQuery] = useState('');
  const [filterText, setFilterText] = useState('');
  const [k, setK] = useState(5);
  const [indexName, setIndexName] = useState('test_index');
  const [dimension, setDimension] = useState(384);
  const [results, setResults] = useState<any[]>([]);
  const [loading, setLoading] = useState(false);
  const [modelLoading, setModelLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSearch = async () => {
    if (!query) return;
    setLoading(true);
    setError(null);
    try {
      let vector: string[] = [];
      
      // Use real AI embeddings if dimension is 384 (MiniLM)
      if (dimension === 384) {
        try {
          if (!extractorPipe) {
            setModelLoading(true);
            // @ts-ignore
            extractorPipe = await pipeline('feature-extraction', 'Xenova/all-MiniLM-L6-v2');
            setModelLoading(false);
          }
          const out = await extractorPipe(query, { pooling: 'mean', normalize: true });
          vector = Array.from(out.data).map(String);
        } catch (err) {
          console.error("Failed to load local model, falling back to random:", err);
          vector = Array.from({length: dimension}, () => (Math.random() * 2 - 1).toFixed(6));
          setModelLoading(false);
        }
      } else {
        // Fallback to random simulation for other dimensions
        vector = Array.from({length: dimension}, () => (Math.random() * 2 - 1).toFixed(6));
      }
      
      const cmd = ['VSEARCH', indexName, ...vector, k.toString(), 'WITHDOCS', `doc:${indexName}`];
      if (filterText.trim().length > 0) {
        cmd.push('FILTER_CONTAINS', filterText.trim());
      }

      const res = await fetchWithAuth(`/t/${clusterId}/query`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ command: cmd })
      });
      
      if (!res.ok) {
        throw new Error(await res.text());
      }
      
      const data = await res.json();
      const respStr = data.response || '';
      
      // Parse RESP Array of Arrays with WITHDOCS
      
      const parsedResults: any[] = [];
      const lines = respStr.split('\r\n');
      
      if (lines[0].startsWith('*')) {
         const numResults = parseInt(lines[0].substring(1));
         let lineIdx = 1;
         for (let i = 0; i < numResults; i++) {
            if (lines[lineIdx] && lines[lineIdx].startsWith('*')) {
               lineIdx++; // skip *3
               
               // Read DocID
               lineIdx++; // skip string length
               const docId = lines[lineIdx++];
               
               // Read Score
               lineIdx++; // skip string length
               const score = parseFloat(lines[lineIdx++]);
               
               // Read Doc string
               lineIdx++; // skip string length
               let metaText = lines[lineIdx++];
               
               try {
                   const parsedMeta = JSON.parse(metaText);
                   metaText = parsedMeta.page_content || JSON.stringify(parsedMeta);
               } catch(e) {
                   // Ignore if not json
               }
               
               parsedResults.push({ id: docId, score: (score * 100).toFixed(2), text: metaText });
            }
         }
      }
      
      setResults(parsedResults);
      // GSAP animate results list
      gsap.fromTo('.result-card',
        { y: 30, opacity: 0 },
        { y: 0, opacity: 1, duration: 0.5, stagger: 0.1, ease: 'back.out(1.7)' }
      );
    } catch (e: any) {
      setError(e.message || "Failed to execute vector search");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="page-shell">
      <div className="content-area">
        <header className="page-header relative z-10">
          <div className="page-title"><Network size={20} /> Vector Playground</div>
          <p style={{color: 'var(--text-muted)', marginTop: 4, fontSize: 14}}>Test Retrieval-Augmented Generation (RAG) directly against the DBX Engine.</p>
        </header>

        <div className="vector-playground-layout relative min-h-[600px]">
          <VectorGalaxy results={results} />
          
          <div className="panel query-panel z-10 relative bg-white/80 dark:bg-slate-900/80 backdrop-blur-md">
            <div className="panel-header">
              <div className="panel-title">Semantic Search</div>
            </div>
            <div className="panel-body" style={{display: 'flex', flexDirection: 'column', gap: 16}}>
              <div>
                <label>Index Name (Namespace)</label>
                <select
                  className="input-field"
                  value={indexName}
                  onChange={e => setIndexName(e.target.value)}
                >
                  <option value="test_index">test_index</option>
                  <option value="quant_knowledge">quant_knowledge</option>
                  <option value="star_output">star_output</option>
                  <option value="bench_vectors">bench_vectors (benchmark data)</option>
                </select>
              </div>
              <div>
                <label>Query Text</label>
                <textarea 
                  className="input-field" 
                  rows={4} 
                  placeholder="Ask a question..."
                  value={query}
                  onChange={e => setQuery(e.target.value)}
                />
              </div>
              <div>
                <label>Metadata Filter (Substring)</label>
                <input 
                  type="text" 
                  className="input-field" 
                  placeholder="e.g. enterprise"
                  value={filterText}
                  onChange={e => setFilterText(e.target.value)}
                />
              </div>
              <div style={{display: 'flex', gap: 16}}>
                  <div style={{flex: 1}}>
                     <label>Embedding Dimension</label>
                     <select 
                       className="input-field" 
                       value={dimension}
                       onChange={e => setDimension(parseInt(e.target.value))}
                     >
                        <option value={384}>384 (HuggingFace MiniLM)</option>
                        <option value={768}>768 (BGE/BERT)</option>
                        <option value={1536}>1536 (OpenAI)</option>
                     </select>
                  </div>
                  <div style={{flex: 1}}>
                     <label>Top K Results</label>
                     <input 
                       type="number" 
                       className="input-field" 
                       value={k}
                       onChange={e => setK(parseInt(e.target.value))}
                       min={1}
                       max={100}
                     />
                  </div>
              </div>
              
              <button className="btn-primary" style={{alignSelf: 'flex-start'}} onClick={handleSearch} disabled={loading || modelLoading}>
                {modelLoading ? (
                   <><Loader2 className="animate-spin" size={16} /> Loading AI Model (1st time only)...</>
                ) : (
                   <><Play size={16} fill="currentColor" /> {loading ? 'Executing...' : 'Run Vector Search'}</>
                )}
              </button>
              
              {error && (
                <div className="alert-error">
                  <AlertTriangle size={16} /> {error}
                </div>
              )}
            </div>
          </div>

          <div className="panel results-panel z-10 relative bg-white/80 dark:bg-slate-900/80 backdrop-blur-md">
            <div className="panel-header">
              <div className="panel-title"><TerminalSquare size={16} /> Search Results</div>
            </div>
            <div className="results-list">
               {results.length === 0 && !loading && (
                  <div className="empty-state">No results to display.</div>
               )}
               
               {results.map((r, i) => (
                  <div key={i} className="result-card">
                     <div className="result-header">
                        <span className="result-rank">#{i+1}</span>
                        <span className="result-score">Similarity: {r.score}%</span>
                     </div>
                     <div className="result-text">{r.text}</div>
                     <div className="result-meta">ID: {r.id}</div>
                  </div>
               ))}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
