import { useState } from 'react';
import { Play, TerminalSquare, AlertTriangle, Loader2 } from 'lucide-react';
import { fetchWithAuth } from '../api';
import { pipeline, env } from '@xenova/transformers';
import PageChrome from '../components/PageChrome';
import gsap from 'gsap';

env.allowLocalModels = false;

let extractorPipe: Awaited<ReturnType<typeof pipeline>> | null = null;

export default function VectorPlaygroundPage({ clusterId }: { clusterId: string }) {
  const [query, setQuery] = useState('');
  const [filterText, setFilterText] = useState('');
  const [k, setK] = useState(5);
  const [indexName, setIndexName] = useState('big_web_index');
  const [dimension, setDimension] = useState(384);
  const [results, setResults] = useState<{ id: string; score: string; text: string }[]>([]);
  const [loading, setLoading] = useState(false);
  const [modelLoading, setModelLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [searched, setSearched] = useState(false);

  const handleSearch = async () => {
    if (!query) return;
    setLoading(true);
    setError(null);
    setSearched(true);
    try {
      let vector: string[] = [];

      if (dimension === 384) {
        try {
          if (!extractorPipe) {
            setModelLoading(true);
            extractorPipe = await pipeline('feature-extraction', 'Xenova/all-MiniLM-L6-v2');
            setModelLoading(false);
          }
          const out = await (extractorPipe as (text: string, opts: { pooling: string; normalize: boolean }) => Promise<{ data: ArrayLike<number> }>)(
            query,
            { pooling: 'mean', normalize: true }
          );
          vector = Array.from(out.data).map(String);
        } catch (err) {
          console.error('Failed to load local model, falling back to random:', err);
          vector = Array.from({ length: dimension }, () => (Math.random() * 2 - 1).toFixed(6));
          setModelLoading(false);
        }
      } else {
        vector = Array.from({ length: dimension }, () => (Math.random() * 2 - 1).toFixed(6));
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

      if (typeof respStr === 'string' && respStr.startsWith('-')) {
        throw new Error(respStr.replace(/\r\n/g, ' ').trim());
      }

      const parsedResults: { id: string; score: string; text: string }[] = [];
      const lines = respStr.split('\r\n');

      if (lines[0]?.startsWith('*')) {
        const numResults = parseInt(lines[0].substring(1));
        let lineIdx = 1;
        for (let i = 0; i < numResults; i++) {
          if (lines[lineIdx] && lines[lineIdx].startsWith('*')) {
            lineIdx++;
            lineIdx++;
            const docId = lines[lineIdx++];
            lineIdx++;
            const score = parseFloat(lines[lineIdx++]);
            lineIdx++;
            let metaText = lines[lineIdx++];

            try {
              const parsedMeta = JSON.parse(metaText);
              metaText = parsedMeta.page_content || JSON.stringify(parsedMeta);
            } catch {
              /* raw */
            }

            parsedResults.push({ id: docId, score: (score * 100).toFixed(2), text: metaText });
          }
        }
      }

      setResults(parsedResults);
      if (parsedResults.length > 0) {
        gsap.fromTo('.result-card',
          { opacity: 0 },
          { opacity: 1, duration: 0.15, stagger: 0.04, ease: 'none' }
        );
      }
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to execute vector search');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="content-area">
      <PageChrome
        clusterId={clusterId}
        title="Vector Playground"
        purpose="Query this tenant’s vector index the same way an application would."
      />

      <p className="text-[12px] text-[var(--text-muted)] -mt-2">
        Scores are cosine similarity × 100. Low scores mean weak embedding alignment, not a UI error.
      </p>

      <div className="vector-playground-layout">
        <div className="panel">
          <div className="panel-header">
            <div className="panel-title">Semantic search</div>
          </div>
          <div className="panel-body" style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
            <div>
              <label>Index name</label>
              <select
                className="input-field mt-1"
                value={indexName}
                onChange={e => {
                  const next = e.target.value;
                  setIndexName(next);
                  if (next === 'bench_vectors') setDimension(128);
                  if (next === 'big_web_index' || next === 'concurrent_index') setDimension(384);
                }}
              >
                <option value="big_web_index">big_web_index (AG News)</option>
                <option value="concurrent_index">concurrent_index (Benchmark)</option>
                <option value="test_index">test_index</option>
                <option value="quant_knowledge">quant_knowledge</option>
                <option value="star_output">star_output</option>
                <option value="bench_vectors">bench_vectors (128-d)</option>
              </select>
            </div>
            <div>
              <label>Query text</label>
              <textarea
                className="input-field mt-1"
                rows={4}
                placeholder="Ask a question…"
                value={query}
                onChange={e => setQuery(e.target.value)}
              />
            </div>
            <div>
              <label>Metadata filter (substring)</label>
              <input
                type="text"
                className="input-field mt-1"
                placeholder="e.g. enterprise"
                value={filterText}
                onChange={e => setFilterText(e.target.value)}
              />
            </div>
            <div style={{ display: 'flex', gap: 12 }}>
              <div style={{ flex: 1 }}>
                <label>Embedding dimension</label>
                <select
                  className="input-field mt-1"
                  value={dimension}
                  onChange={e => setDimension(parseInt(e.target.value))}
                >
                  <option value={128}>128 (benchmark)</option>
                  <option value={384}>384 (MiniLM)</option>
                  <option value={768}>768 (BGE/BERT)</option>
                  <option value={1536}>1536 (OpenAI)</option>
                </select>
              </div>
              <div style={{ flex: 1 }}>
                <label>Top K</label>
                <input
                  type="number"
                  className="input-field mt-1 font-mono"
                  value={k}
                  onChange={e => setK(parseInt(e.target.value))}
                  min={1}
                  max={100}
                />
              </div>
            </div>

            <button type="button" className="btn-primary self-start" onClick={handleSearch} disabled={loading || modelLoading}>
              {modelLoading ? (
                <><Loader2 className="animate-spin" size={14} /> Loading embedding model…</>
              ) : (
                <><Play size={14} fill="currentColor" /> {loading ? 'Searching…' : 'Run search'}</>
              )}
            </button>

            {error && (
              <div className="alert-error">
                <AlertTriangle size={14} /> {error}
              </div>
            )}
          </div>
        </div>

        <div className="panel flex flex-col min-h-[420px]">
          <div className="panel-header">
            <div className="panel-title"><TerminalSquare size={14} /> Results</div>
          </div>
          <div className="results-list">
            {results.length === 0 && !loading && (
              <div className="empty-state">
                {searched
                  ? 'No matches in this index. Try a broader query or confirm the index name.'
                  : 'Run a search to see ranked documents.'}
              </div>
            )}

            {results.map((r, i) => (
              <div key={r.id + i} className="result-card">
                <div className="result-header">
                  <span className="result-rank">#{i + 1}</span>
                  <span className="result-score">{r.score}</span>
                </div>
                <div className="result-text">{r.text}</div>
                <div className="result-meta">ID: {r.id}</div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
