import { engineLabel, voiceCreditLine, voiceCreditSummary } from '../voiceCredit';

it('names known engines and passes others through', () => {
  expect(engineLabel('voicevox:3')).toBe('VOICEVOX');
  expect(engineLabel('coeiroink:12')).toBe('coeiroink');
  expect(engineLabel('')).toBeUndefined();
  expect(engineLabel(undefined)).toBeUndefined();
});

it('credits the voice with or without an engine', () => {
  expect(voiceCreditLine('ずんだもん', 'voicevox:3')).toBe('VOICEVOX · ずんだもん');
  expect(voiceCreditLine('Alice', undefined)).toBe('Alice');
});

it('summarises the deck by engine', () => {
  expect(voiceCreditSummary([
    { voice: 'ずんだもん', source: 'voicevox:3' },
    { voice: '四国めたん', source: 'voicevox:2' },
    { voice: 'Alice', source: 'kokoro:af' },
    { voice: 'Bob' },
  ])).toBe('VOICEVOX (ずんだもん, 四国めたん); kokoro (Alice); Other (Bob)');
});
