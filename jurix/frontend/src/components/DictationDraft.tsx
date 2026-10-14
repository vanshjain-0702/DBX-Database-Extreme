import { useEffect, useRef, useState } from 'react';
import { Mic, Square, Upload } from 'lucide-react';
import { api, message } from '../lib/api';
import { useSession } from '../lib/session';
import { useMatter } from '../pages/MatterLayout';
import { ErrorNotice } from './Feedback';
export default function DictationDraft({onConfirm, disabled}: {onConfirm: (text: string) => void; disabled?: boolean}) {
    const {matter} = useMatter();
    const {capabilities} = useSession();
    const [recording, setRecording] = useState(false);
    const [busy, setBusy] = useState(false);
    const [audio, setAudio] = useState<File | null>(null);
    const [transcript, setTranscript] = useState('');
    const [draft, setDraft] = useState('');
    const [lexicon, setLexicon] = useState('');
    const [language, setLanguage] = useState('');
    const [error, setError] = useState('');
    const [warnings, setWarnings] = useState<string[]>([]);
    const recorder = useRef<MediaRecorder | null>(null);
    const stream = useRef<MediaStream | null>(null);
    const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
    const active = useRef(true);
    useEffect(() => {
        active.current = true;
        return () => { active.current = false; if (timer.current) clearTimeout(timer.current); if (recorder.current?.state === 'recording') recorder.current.stop(); stream.current?.getTracks().forEach(track => track.stop()); };
    }, []);
    const available = !!capabilities?.dictation?.available && !!matter.external_processing && !disabled;
    async function start() {
        setError(''); setAudio(null); setTranscript(''); setDraft('');
        try {
            if (!navigator.mediaDevices?.getUserMedia || !window.MediaRecorder) throw new Error('Recording requires a supported browser and a secure connection. Upload an audio file instead.');
            stream.current = await navigator.mediaDevices.getUserMedia({audio: true});
            if (!active.current) { stream.current.getTracks().forEach(track => track.stop()); return; }
            const chunks: Blob[] = [];
            const mime = ['audio/webm;codecs=opus', 'audio/webm', 'audio/mp4'].find(type => MediaRecorder.isTypeSupported(type));
            if (!mime) throw new Error('Your browser cannot record a supported format. Upload a WAV, MP3, MP4, M4A or WebM audio file.');
            const next = new MediaRecorder(stream.current, {mimeType: mime}); recorder.current = next;
            next.ondataavailable = event => { if (event.data.size) chunks.push(event.data); };
            next.onstop = () => {
                stream.current?.getTracks().forEach(track => track.stop());
                if (timer.current) clearTimeout(timer.current);
                if (!active.current) return;
                const type = next.mimeType || 'audio/webm';
                setAudio(new File(chunks, `review-note.${type.includes('mp4') ? 'mp4' : 'webm'}`, {type})); setRecording(false);
            };
            next.start(); setRecording(true);
            timer.current = setTimeout(() => { if (next.state === 'recording') next.stop(); }, 180000);
        } catch (error) { stream.current?.getTracks().forEach(track => track.stop()); setError(message(error)); setRecording(false); }
    }
    async function transcribe() {
        if (!audio || !available) return; setBusy(true); setError(''); setWarnings([]);
        try {
            if (audio.size > 20 * 1024 * 1024) throw new Error('Audio must be no larger than 20 MB.');
            if (!/\.(wav|mp3|mp4|mpeg|mpga|m4a|webm)$/i.test(audio.name)) throw new Error('Choose WAV, MP3, MP4, MPEG, MPGA, M4A or WebM audio.');
            const body = new FormData(); body.append('file', audio); body.append('lexicon', lexicon); if (language.trim()) body.append('language', language.trim());
            const result = await api<{transcript: string; structured_note: string | object; warnings: string[]}>(`/api/matters/${matter.id}/dictation`, {method: 'POST', body});
            if (!active.current) return;
            setTranscript(result.transcript); setDraft(typeof result.structured_note === 'string' ? result.structured_note : JSON.stringify(result.structured_note, null, 2)); setWarnings(result.warnings || []); setAudio(null);
        } catch (error) { setError(message(error)); } finally { setBusy(false); }
    }
    return <details className="dictation-draft"><summary><Mic size={15}/>Dictate a review note</summary><div className="form-stack"><p className="field-help">Create an editable draft with your firm's terminology. Confirm the wording before saving a note. JURIX does not retain audio after processing.</p>
        {!capabilities ? <p className="notice">Provider availability is unverified. Check Firm & connection before using dictation.</p> : !capabilities.dictation?.available ? <p className="notice">Dictation provider is not configured. Your administrator can configure it on the server; see Firm & connection.</p> : !matter.external_processing && <p className="notice">Enable external processing for this matter before submitting audio.</p>}
        <ErrorNotice error={error}/><div className="form-grid"><div><label htmlFor="dictation-lexicon">Firm terminology / names</label><input id="dictation-lexicon" value={lexicon} maxLength={4000} disabled={!available || busy || recording} onChange={event => setLexicon(event.target.value)} placeholder="Client names, clause terms, abbreviations"/></div><div><label htmlFor="dictation-language">Language (optional)</label><input id="dictation-language" value={language} maxLength={80} disabled={!available || busy || recording} onChange={event => setLanguage(event.target.value)} placeholder="e.g. English"/></div></div>
        <div className="inline-actions">{recording ? <button type="button" className="button recording" onClick={() => recorder.current?.stop()}><Square size={15}/>Stop recording</button> : <button type="button" className="button subtle" onClick={start} disabled={!available || busy}><Mic size={15}/>Record note</button>}<label className="button subtle file-button"><Upload size={15}/>Upload audio<input type="file" accept=".wav,.mp3,.mp4,.mpeg,.mpga,.m4a,.webm" disabled={!available || busy || recording} onChange={event => {setAudio(event.target.files?.[0] || null); setError(''); event.target.value = '';}}/></label></div>{recording && <p className="notice warning" role="status">Microphone is recording. Recording stops automatically after three minutes.</p>}{audio && <div className="inline-actions"><span className="field-help">{audio.name} · {(audio.size / 1024).toFixed(0)} KB</span><button type="button" className="button primary" disabled={!available || busy || recording} onClick={transcribe}>{busy ? 'Transcribing…' : 'Prepare editable draft'}</button></div>}
        {warnings.map((warning,index) => <p className="notice warning" key={index}>{warning}</p>)}{transcript && <div><label htmlFor="dictation-transcript">Transcript draft</label><textarea id="dictation-transcript" rows={4} value={transcript} onChange={event => setTranscript(event.target.value)}/></div>}{draft && <div><label htmlFor="dictation-note">Review note draft</label><textarea id="dictation-note" rows={4} value={draft} onChange={event => setDraft(event.target.value)}/><div className="inline-actions"><button type="button" className="button subtle" disabled={!draft.trim() || disabled} onClick={() => {onConfirm(draft.trim()); setDraft(''); setTranscript('');}}>Confirm draft into note editor</button><button type="button" className="text-button" onClick={() => setDraft(transcript)}>Use edited transcript</button></div></div>}
    </div></details>;
}
