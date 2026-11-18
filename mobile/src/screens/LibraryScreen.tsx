import { useState } from 'react';
import { StyleSheet, View } from 'react-native';
import { ScreenHeader } from '../components/ScreenHeader';
import { SegmentedControl } from '../components/SegmentedControl';
import SuggestedScreen from './SuggestedScreen';
import VocabScreen from './VocabScreen';

type Segment = 'words' | 'suggested';

const SEGMENTS: { key: Segment; label: string }[] = [
  { key: 'words', label: 'Words' },
  { key: 'suggested', label: 'Suggested' },
];

/**
 * Library tab: your words and the tutor's suggestions. Both sub-screens stay
 * mounted (toggled via `display`) so search/scroll state survives switching.
 * Stats live behind a row in Settings.
 */
export default function LibraryScreen() {
  const [segment, setSegment] = useState<Segment>('words');

  return (
    <View style={styles.root}>
      {/* Big title like every other tab — Library was the only one that
          jumped straight into content. Sub-screens keep their subtitle-only
          headers (word count / stats period) below the segmented control. */}
      <ScreenHeader title="Library" />
      <SegmentedControl segments={SEGMENTS} active={segment} onChange={setSegment} />
      <View style={[styles.pane, segment === 'words' ? styles.shown : styles.hidden]}>
        <VocabScreen />
      </View>
      <View style={[styles.pane, segment === 'suggested' ? styles.shown : styles.hidden]}>
        <SuggestedScreen />
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  root: {
    flex: 1,
  },
  pane: {
    flex: 1,
  },
  shown: {
    display: 'flex',
  },
  hidden: {
    display: 'none',
  },
});
