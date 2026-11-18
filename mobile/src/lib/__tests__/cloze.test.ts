import { buildClozeParts, clozeTarget } from '../cloze';

it('blanks the surface form first, every occurrence', () => {
  const parts = buildClozeParts('雪が解けて、また解けた。', ['解け', '解ける']);
  expect(parts).toEqual([
    { kind: 'text', text: '雪が' },
    { kind: 'blank', text: '解け' },
    { kind: 'text', text: 'て、また' },
    { kind: 'blank', text: '解け' },
    { kind: 'text', text: 'た。' },
  ]);
  expect(clozeTarget(parts)).toBe('解け');
});

it('falls back through the candidates and then to plain text', () => {
  expect(buildClozeParts('問題が解けた。', [undefined, '解ける', '解け'])).toEqual([
    { kind: 'text', text: '問題が' },
    { kind: 'blank', text: '解け' },
    { kind: 'text', text: 'た。' },
  ]);
  const none = buildClozeParts('関係ない文。', ['解ける']);
  expect(none).toEqual([{ kind: 'text', text: '関係ない文。' }]);
  expect(clozeTarget(none)).toBeUndefined();
});
