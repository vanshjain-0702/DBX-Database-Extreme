import { useState } from 'react';
import { Play, TerminalSquare, AlertTriangle, Loader2 } from 'lucide-react';
import { fetchWithAuth } from '../api';
import { pipeline, env } from '@xenova/transformers';
import PageChrome from '../components/PageChrome';
import gsap from 'gsap';

env.allowLocalModels = false;

let extractorPipe: Awaited<ReturnType<typeof pipeline>> | null = null;

type SearchMode = 'semantic' | 'similar' | 'multimodal';

type Hit = { id: string; score: string; text: string };

const MINILM_DIM = 384;

async function encodeText(
  text: string,
  onLoad: (loading: boolean) => void
): Promise<string[]> {
  if (!extractorPipe) {
    onLoad(true);
    try {
      extractorPipe = await pipeline('feature-extraction', 'Xenova/all-MiniLM-L6-v2');
    } finally {
      onLoad(false);
    }
  }
  const out = await (extractorPipe as (input: string, opts: { pooling: string; normalize: boolean }) => Promise<{ data: ArrayLike<number> }>)(
    text,
    { pooling: 'mean', normalize: true }
  );
  return Array.from(out.data).map(String);
}

function parseHits(respStr: string): Hit[] {
  const parsedResults: Hit[] = [];
  const lines = respStr.split('\r\n');
  if (!lines[0]?.startsWith('*')) {
    return parsedResults;
  }
  const numResults = parseInt(lines[0].substring(1), 10);
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
  return parsedResults;
}

export default function VectorPlaygroundPage({ clusterId }: { clusterId: string }) {
  const [mode, setMode] = useState<SearchMode>('semantic');
  const [query, setQuery] = useState('');
  const [secondQuery, setSecondQuery] = useState('');
  const [similarId, setSimilarId] = useState('');
  const [filterText, setFilterText] = useState('');
  const [k, setK] = useState(5);
  const [minScore, setMinScore] = useState('');
  const [ef, setEf] = useState('');
  const [space, setSpace] = useState('');
  const [spaceA, setSpaceA] = useState('text');
  const [spaceB, setSpaceB] = useState('image');
  const [weightA, setWeightA] = useState('0.5');
  const [weightB, setWeightB] = useState('0.5');
  const [indexName, setIndexName] = useState('big_web_index');
  const [results, setResults] = useState<Hit[]>([]);
  const [loading, setLoading] = useState(false);
  const [modelLoading, setModelLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [searched, setSearched] = useState(false);

  const appendCommonFlags = (cmd: string[]) => {
    cmd.push('WITHDOCS', `doc:${indexName}`);
    if (filterText.trim().length > 0) {
      cmd.push('FILTER_CONTAINS', filterText.trim());
    }
    if (minScore.trim().length > 0) {
      cmd.push('MIN_SCORE', minScore.trim());
    }
    if (ef.trim().length > 0) {
      cmd.push('EF', ef.trim());
    }
  };

  const handleSearch = async () => {
    setLoading(true);
    setError(null);
    setSearched(true);
    try {
      let cmd: string[];
      if (mode === 'similar') {
        if (!similarId.trim()) {
          throw new Error('Enter a stored vector id to find neighbors.');
        }
        cmd = ['VSIM', indexName, similarId.trim(), k.toString()];
        if (space.trim()) {
          cmd.push('SPACE', space.trim());
        }
        appendCommonFlags(cmd);
      } else if (mode === 'multimodal') {
        if (!query.trim() || !secondQuery.trim()) {
          throw new Error('Enter a query for both spaces. DBX fuses caller embeddings; it does not run CLIP.');
        }
        const vecA = await encodeText(query.trim(), setModelLoading);
        const vecB = await encodeText(secondQuery.trim(), setModelLoading);
        cmd = [
          'VFUSE', indexName,
          'SPACE', spaceA.trim() || 'text', ...vecA,
          'SPACE', spaceB.trim() || 'image', ...vecB,
          k.toString(),
          'WEIGHTS', `${weightA || '0.5'},${weightB || '0.5'}`,
        ];
        appendCommonFlags(cmd);
      } else {
        if (!query.trim()) {
          throw new Error('Enter query text to embed in the browser.');
        }
        const vector = await encodeText(query.trim(), setModelLoading);
        cmd = ['VSEARCH', indexName, ...vector, k.toString()];
        if (space.trim()) {
          cmd.push('SPACE', space.trim());
        }
        appendCommonFlags(cmd);
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

      const parsedResults = parseHits(respStr);
      setResults(parsedResults);
      if (parsedResults.length > 0) {
        gsap.fromTo('.result-card',
          { opacity: 0 },
          { opacity: 1, duration: 0.15, stagger: 0.04, ease: 'none' }
        );
      }
    } catch (e: unknown) {
      setResults([]);
      setError(e instanceof Error ? e.message : 'Failed to execute vector search');
    } finally {
      setLoading(false);
    }
  };

  const scoreHint = mode === 'multimodal'
    ? 'Scores are a weighted sum of per-space cosine × 100. DBX stores the floats you send; embeddings stay in this browser.'
    : 'Scores are cosine similarity × 100. Embeddings are computed in this browser with MiniLM (384-d). DBX does not run a model.';

  const title = mode === 'similar' ? 'Similarity search' : mode === 'multimodal' ? 'Multimodal fusion' : 'Semantic search';

  return (
    <div className="content-area">
      <PageChrome
        clusterId={clusterId}
        title="Vector Playground"
        purpose="Query this tenant’s vector index the same way an application would."
      />

      <p className="text-[12px] text-[var(--text-muted)] -mt-2">
        {scoreHint}
      </p>

      <div className="vector-playground-layout">
        <div className="panel">
          <div className="panel-header">
            <div className="panel-title">{title}</div>
          </div>
          <div className="panel-body" style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
            <div className="search-mode-tabs" role="tablist">
              <button type="button" className={mode === 'semantic' ? 'is-on' : ''} onClick={() => setMode('semantic')}>Semantic</button>
              <button type="button" className={mode === 'similar' ? 'is-on' : ''} onClick={() => setMode('similar')}>Similar-to-id</button>
              <button type="button" className={mode === 'multimodal' ? 'is-on' : ''} onClick={() => setMode('multimodal')}>Multimodal</button>
            </div>

            <div>
              <label>Index name</label>
              <select
                className="input-field mt-1"
                value={indexName}
                onChange={e => setIndexName(e.target.value)}
              >
                <option value="big_web_index">big_web_index (AG News)</option>
                <option value="concurrent_index">concurrent_index (Benchmark)</option>
                <option value="test_index">test_index</option>
                <option value="quant_knowledge">quant_knowledge</option>
                <option value="star_output">star_output</option>
                <option value="bench_vectors">bench_vectors (128-d — use console)</option>
              </select>
            </div>

            {mode === 'similar' ? (
              <div>
                <label>Stored vector id</label>
                <input
                  type="text"
                  className="input-field mt-1 font-mono"
                  placeholder="doc id already in this index"
                  value={similarId}
                  onChange={e => setSimilarId(e.target.value)}
                />
              </div>
            ) : (
              <div>
                <label>{mode === 'multimodal' ? `Query for space “${spaceA || 'text'}”` : 'Query text'}</label>
                <textarea
                  className="input-field mt-1"
                  rows={3}
                  placeholder={mode === 'multimodal' ? 'Text you embedded into the first space…' : 'Ask a question…'}
                  value={query}
                  onChange={e => setQuery(e.target.value)}
                />
              </div>
            )}

            {mode === 'multimodal' && (
              <div>
                <label>Query for space “{spaceB || 'image'}”</label>
                <textarea
                  className="input-field mt-1"
                  rows={3}
                  placeholder="Second modality as text here (your app sends CLIP floats)"
                  value={secondQuery}
                  onChange={e => setSecondQuery(e.target.value)}
                />
              </div>
            )}

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

            {mode === 'multimodal' ? (
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                <div>
                  <label>Space A</label>
                  <input className="input-field mt-1 font-mono" value={spaceA} onChange={e => setSpaceA(e.target.value)} />
                </div>
                <div>
                  <label>Space B</label>
                  <input className="input-field mt-1 font-mono" value={spaceB} onChange={e => setSpaceB(e.target.value)} />
                </div>
                <div>
                  <label>Weight A</label>
                  <input className="input-field mt-1 font-mono" value={weightA} onChange={e => setWeightA(e.target.value)} />
                </div>
                <div>
                  <label>Weight B</label>
                  <input className="input-field mt-1 font-mono" value={weightB} onChange={e => setWeightB(e.target.value)} />
                </div>
              </div>
            ) : (
              <div>
                <label>Space (optional)</label>
                <input
                  type="text"
                  className="input-field mt-1 font-mono"
                  placeholder="default index"
                  value={space}
                  onChange={e => setSpace(e.target.value)}
                />
              </div>
            )}

            <div style={{ display: 'flex', gap: 12 }}>
              <div style={{ flex: 1 }}>
                <label>Top K</label>
                <input
                  type="number"
                  className="input-field mt-1 font-mono"
                  value={k}
                  onChange={e => setK(parseInt(e.target.value, 10) || 1)}
                  min={1}
                  max={100}
                />
              </div>
              <div style={{ flex: 1 }}>
                <label>Min score</label>
                <input
                  type="text"
                  className="input-field mt-1 font-mono"
                  placeholder="e.g. 0.35"
                  value={minScore}
                  onChange={e => setMinScore(e.target.value)}
                />
              </div>
            </div>
            <div>
              <label>EF (optional HNSW breadth)</label>
              <input
                type="text"
                className="input-field mt-1 font-mono"
                placeholder="engine default 80"
                value={ef}
                onChange={e => setEf(e.target.value)}
              />
            </div>
            <p className="text-[11px] text-[var(--text-muted)]">
              Browser encoder is MiniLM at {MINILM_DIM} dimensions. 128-d benchmark indexes will reject the query — use the console with matching floats.
            </p>

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
                  ? 'No matches in this index. Try a broader query, a lower min score, or confirm the index and space names.'
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
