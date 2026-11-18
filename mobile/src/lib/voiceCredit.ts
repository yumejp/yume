import type { MediaVoice } from '../api/types';

/** Engines whose credit wording differs from their source id. Anything else
 *  is shown as written before the colon in the clip's source. */
const ENGINE_LABELS: Record<string, string> = { voicevox: 'VOICEVOX' };

export function engineLabel(source?: string | null): string | undefined {
  const engine = source?.split(':')[0]?.trim();
  if (!engine) return undefined;
  return ENGINE_LABELS[engine.toLowerCase()] ?? engine;
}

/** "VOICEVOX · ずんだもん", or just the voice when the engine is unknown. */
export function voiceCreditLine(voice: string, source?: string | null): string {
  const engine = engineLabel(source);
  return engine ? `${engine} · ${voice}` : voice;
}

/** Every credited voice grouped by engine: "VOICEVOX (ずんだもん, 四国めたん)". */
export function voiceCreditSummary(voices: Pick<MediaVoice, 'voice' | 'source'>[]): string {
  const byEngine = new Map<string, string[]>();
  for (const v of voices) {
    const engine = engineLabel(v.source) ?? 'Other';
    const names = byEngine.get(engine) ?? [];
    if (!names.includes(v.voice)) names.push(v.voice);
    byEngine.set(engine, names);
  }
  return [...byEngine].map(([engine, names]) => `${engine} (${names.join(', ')})`).join('; ');
}
