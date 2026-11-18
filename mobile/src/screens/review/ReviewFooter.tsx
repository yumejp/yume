import { useEffect, useRef, useState } from 'react';
import { ActivityIndicator, Pressable, StyleSheet, Text, View } from 'react-native';
import { Icon } from '../../components/Icon';
import type { ReviewCardVM } from '../../data/live';
import { spokenInterval } from '../../lib/format';
import { colors, font, spacing } from '../../theme';
import {
  RATINGS,
  type Phase,
  type Rating,
  type RatingReceipt,
} from '../useReviewSession';
import { Fade } from './anim';
import { SHOW_KEY_HINTS } from './CardFace';
import { styles } from './ReviewScreen.styles';
import { haptic } from './shared';

interface Props {
  phase: Phase;
  card: ReviewCardVM;
  mode: 'live' | 'sample' | 'offline' | null;
  error: string | null;
  notice: string | null;
  lastRated: RatingReceipt | null;
  canUndo: boolean;
  submitting: boolean;
  pendingRating: Rating | null;
  againRepeats: boolean;
  rate: (rating: (typeof RATINGS)[number]) => Promise<void>;
  undo: () => Promise<void>;
  reveal: () => void;
}

export function ReviewFooter({
  phase,
  card,
  mode,
  error,
  notice,
  lastRated,
  canUndo,
  submitting,
  pendingRating,
  againRepeats,
  rate,
  undo,
  reveal,
}: Props) {
  const ratedRef = useRef(lastRated);
  if (lastRated) ratedRef.current = lastRated;
  const receipt = lastRated ?? ratedRef.current;
  const grad = receipt?.grad ?? null;

  const [slowSave, setSlowSave] = useState(false);
  useEffect(() => {
    if (!submitting) {
      setSlowSave(false);
      return;
    }
    const timer = setTimeout(() => setSlowSave(true), 400);
    return () => clearTimeout(timer);
  }, [submitting]);

  return (
    <>
      {error ? (
        <View style={styles.errorBanner} accessibilityRole="alert">
          <Icon name="alert-circle-outline" size={16} color={colors.statusUnknown} />
          <Text style={styles.errorBannerText}>{error}</Text>
        </View>
      ) : notice ? (
        <View style={[styles.errorBanner, styles.noticeBanner]} accessibilityLiveRegion="polite">
          <Icon name="repeat" size={16} color={colors.textSecondary} />
          <Text style={[styles.errorBannerText, styles.noticeBannerText]}>{notice}</Text>
        </View>
      ) : null}

      <Fade
        visible={!!lastRated}
        style={[
          styles.toast,
          grad === 'promoted' && styles.gradToast,
          grad === 'demoted' && styles.gradToastDemote,
        ]}
      >
        {grad ? (
          <Icon
            name={grad === 'promoted' ? 'star-face' : 'restore'}
            size={16}
            color={grad === 'promoted' ? colors.statusKnown : colors.statusLearning}
          />
        ) : null}
        <Text style={styles.toastMuted}>
          Rated {receipt?.label}
          {grad ? (
            <Text style={styles.toastText}>
              {' · '}{receipt?.lemma} → {grad === 'promoted' ? 'Known' : 'Learning'}
            </Text>
          ) : null}
        </Text>
        {canUndo ? (
          <Pressable
            onPress={() => { haptic('tap'); void undo(); }}
            disabled={submitting}
            hitSlop={10}
            style={({ pressed }) => [styles.toastUndo, pressed && styles.pressed]}
            accessibilityRole="button"
            accessibilityLabel="Undo last rating"
          >
            {slowSave && submitting && !pendingRating ? (
              <ActivityIndicator size={14} color={colors.accentText} />
            ) : (
              <Icon name="undo-variant" size={14} color={colors.accentText} />
            )}
            <Text style={styles.toastUndoText}>Undo</Text>
          </Pressable>
        ) : null}
      </Fade>

      <View style={styles.ratings}>
        {phase === 'back' ? (
          RATINGS.map((rating, index) => {
            const saving = slowSave && submitting && pendingRating === rating.key;
            const when =
              card.intervals == null
                ? null
                : rating.key === 'again' && againRepeats && mode !== 'sample'
                  ? 'soon'
                  : card.intervals[rating.key];
            return (
              <Pressable
                key={rating.key}
                style={({ pressed }) => [
                  styles.ratingBtn,
                  (pressed || pendingRating === rating.key) && styles.ratingBtnPressed,
                  submitting && pendingRating !== rating.key && styles.ratingBtnDimmed,
                ]}
                onPress={() => { haptic('tap'); void rate(rating); }}
                disabled={submitting}
                accessibilityRole="button"
                accessibilityLabel={
                  when != null
                    ? `${rating.label}, ${when === 'soon' ? 'comes back this session' : spokenInterval(when)}`
                    : rating.label
                }
              >
                <Text style={[
                  styles.ratingLabel,
                  rating.key === 'again' && styles.ratingLabelAgain,
                  saving && styles.savingDim,
                ]}>
                  {rating.label}
                </Text>
                {when != null ? (
                  <Text style={[styles.ratingWhen, saving && styles.savingDim]}>{when}</Text>
                ) : null}
                {SHOW_KEY_HINTS ? (
                  <Text style={[styles.ratingKey, saving && styles.savingDim]}>{index + 1}</Text>
                ) : null}
                {saving ? (
                  <View style={styles.savingOverlay}>
                    <ActivityIndicator size="small" color={colors.accentText} />
                  </View>
                ) : null}
              </Pressable>
            );
          })
        ) : (
          <Pressable
            style={({ pressed }) => [styles.revealBtn, pressed && styles.pressed]}
            onPress={() => { haptic('select'); reveal(); }}
            accessibilityRole="button"
          >
            <Text style={styles.revealLabel}>Show answer</Text>
          </Pressable>
        )}
      </View>
    </>
  );
}

