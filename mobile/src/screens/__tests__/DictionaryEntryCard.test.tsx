import { render } from '@testing-library/react-native';
import { DictionaryEntryCard } from '../DictionaryEntryCard';
import { EXAMPLES_CREDIT } from '../../lib/credits';

jest.mock('../../settings/SettingsContext', () => ({ useTextScale: () => 1 }));

const entry = { id: 1, kanji: ['猫'], readings: ['ねこ'], senses: [{ pos: ['n'], glosses: ['cat'] }] };
const noop = () => {};

function card(examples: { text: string; translation: string }[] | undefined) {
  return render(
    <DictionaryEntryCard
      entry={entry}
      expanded
      examples={examples}
      examplesLoading={false}
      examplesFailed={false}
      saved={false}
      saving={false}
      onToggleExamples={noop}
      onSave={noop}
      onRemove={noop}
    />,
  );
}

it('credits the Tanaka Corpus under corpus examples', () => {
  const view = card([{ text: '猫が好きです。', translation: 'I like cats.' }]);
  expect(view.getByText('I like cats.')).toBeTruthy();
  expect(view.getByText(EXAMPLES_CREDIT)).toBeTruthy();
});

it('shows no example credit when there are no examples', () => {
  const view = card([]);
  expect(view.getByText('No example sentences.')).toBeTruthy();
  expect(view.queryByText(EXAMPLES_CREDIT)).toBeNull();
});
