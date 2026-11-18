import { act, fireEvent, render } from '@testing-library/react-native';
import { Linking } from 'react-native';
import { CARD_HELP_ACTIONS, CardHelpSheet, TUTOR_GUIDE_URL, type CardHelpContent } from '../CardHelpSheet';

const CONTENT: CardHelpContent = {
  sentence: '猫が寝ている。',
  translation: 'The cat is sleeping.',
  explanation: '',
  mnemonicParts: [{ kanji: '猫', meaning: 'cat' }],
  mnemonicStory: '',
  mnemonicShared: false,
};

function renderSheet(props: Partial<Parameters<typeof CardHelpSheet>[0]> = {}) {
  return render(
    <CardHelpSheet
      visible
      lemma="猫"
      isRequested={() => false}
      savingTag={null}
      error={null}
      onToggle={() => {}}
      onClose={() => {}}
      content={CONTENT}
      onSaveEdit={async () => true}
      editSaving={false}
      editError={null}
      {...props}
    />,
  );
}

it('offers only bounded card-scoped help actions', () => {
  const onToggle = jest.fn();
  const view = renderSheet({ isRequested: (tag) => tag === 'needs-audio', onToggle });

  expect(CARD_HELP_ACTIONS.map((action) => action.tag)).toEqual([
    'needs-explanation',
    'needs-sentence',
    'needs-audio',
    'needs-fix',
  ]);
  expect(view.getByText('Requested')).toBeTruthy();
  expect(view.queryByPlaceholderText(/ask|chat|message/i)).toBeNull();

  fireEvent.press(view.getByText('Explain this word'));
  expect(onToggle).toHaveBeenCalledWith('needs-explanation');
});

it('says a request waits for a tutor and links to the tutor guide', () => {
  const openURL = jest.spyOn(Linking, 'openURL').mockResolvedValue(true);
  const view = renderSheet();

  expect(view.getByText(/Yume does not contact a tutor by itself/)).toBeTruthy();
  fireEvent.press(view.getByText('How to connect a tutor'));
  expect(openURL).toHaveBeenCalledWith(TUTOR_GUIDE_URL);
  openURL.mockRestore();
});

it('writes the learner’s own mnemonic story', async () => {
  const onSaveEdit = jest.fn().mockResolvedValue(true);
  const view = renderSheet({ onSaveEdit });

  fireEvent.press(view.getByText('Write my own mnemonic'));
  expect(view.getByText('猫 cat')).toBeTruthy();
  fireEvent.changeText(view.getByLabelText('Your mnemonic'), 'A cat naps on the grass.');
  await act(async () => { fireEvent.press(view.getByText('Save')); });

  expect(onSaveEdit).toHaveBeenCalledWith({ kind: 'mnemonic', story: 'A cat naps on the grass.' });
  expect(view.getByText('Saved to this card.')).toBeTruthy();
});

it('adds an example with its English', async () => {
  const onSaveEdit = jest.fn().mockResolvedValue(true);
  const view = renderSheet({ onSaveEdit });

  fireEvent.press(view.getByText('Add my own example'));
  fireEvent.changeText(view.getByLabelText('Japanese sentence'), '猫が好きです。');
  fireEvent.changeText(view.getByLabelText('English of the sentence'), 'I like cats.');
  await act(async () => { fireEvent.press(view.getByText('Save')); });

  expect(onSaveEdit).toHaveBeenCalledWith({ kind: 'example', sentence: '猫が好きです。', translation: 'I like cats.' });
});

it('edits the translation and explanation, starting from the card', async () => {
  const onSaveEdit = jest.fn().mockResolvedValue(true);
  const view = renderSheet({ onSaveEdit });

  fireEvent.press(view.getByText('Fix the translation or explanation'));
  expect(view.getByDisplayValue('The cat is sleeping.')).toBeTruthy();
  fireEvent.changeText(view.getByLabelText('Explanation'), 'Casual; 寝る here means sleeping, not lying down.');
  await act(async () => { fireEvent.press(view.getByText('Save')); });

  expect(onSaveEdit).toHaveBeenCalledWith({
    kind: 'meaning',
    translation: 'The cat is sleeping.',
    explanation: 'Casual; 寝る here means sleeping, not lying down.',
  });
});

it('offers only the explanation when the card has no sentence', () => {
  const view = renderSheet({ content: { ...CONTENT, sentence: '', translation: '' } });

  fireEvent.press(view.getByText('Write an explanation'));
  expect(view.queryByLabelText('English of the sentence')).toBeNull();
  expect(view.getByLabelText('Explanation')).toBeTruthy();
});

it('says an example becomes the sentence of a card without one', async () => {
  const view = renderSheet({ content: { ...CONTENT, sentence: '', translation: '' } });

  fireEvent.press(view.getByText('Add my own example'));
  fireEvent.changeText(view.getByLabelText('Japanese sentence'), '猫がいる。');
  await act(async () => { fireEvent.press(view.getByText('Save')); });

  expect(view.getByText('Example added. It shows as this card’s sentence.')).toBeTruthy();
});

it('keeps the draft and shows the error when a save fails', async () => {
  const onSaveEdit = jest.fn().mockResolvedValue(false);
  const view = renderSheet({ onSaveEdit, editError: 'Couldn’t save to this card. Can’t reach the server.' });

  fireEvent.press(view.getByText('Add my own example'));
  fireEvent.changeText(view.getByLabelText('Japanese sentence'), '猫がいる。');
  await act(async () => { fireEvent.press(view.getByText('Save')); });

  expect(view.getByDisplayValue('猫がいる。')).toBeTruthy();
  expect(view.getByText(/Can’t reach the server/)).toBeTruthy();
});
