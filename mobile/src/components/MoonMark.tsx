import { useEffect, useRef, useState } from 'react';
import { AccessibilityInfo, Animated, StyleSheet, View } from 'react-native';
import { colors, radius, spacing } from '../theme';
import { MoonProgress } from './Icon';

const SYNODIC_DAYS = 29.530588853;
const NEW_MOON_REF_MS = 947182440000; // 2000-01-06 18:14 UTC

/** Tonight's moon: illuminated fraction and whether it is waxing. */
export function lunarPhase(now = Date.now()): { f: number; waxing: boolean } {
  const age = (((now - NEW_MOON_REF_MS) / 86_400_000) % SYNODIC_DAYS + SYNODIC_DAYS) % SYNODIC_DAYS;
  const p = age / SYNODIC_DAYS;
  return { f: (1 - Math.cos(2 * Math.PI * p)) / 2, waxing: p < 0.5 };
}

function useReduceMotion(): boolean {
  const [reduce, setReduce] = useState(false);
  useEffect(() => {
    AccessibilityInfo.isReduceMotionEnabled().then(setReduce).catch(() => {});
    const sub = AccessibilityInfo.addEventListener('reduceMotionChanged', setReduce);
    return () => sub.remove();
  }, []);
  return reduce;
}

const STARS = [
  { x: -78, y: -30, s: 3, d: 3200, delay: 0 },
  { x: -52, y: 34, s: 2, d: 4100, delay: 900 },
  { x: 66, y: -42, s: 3, d: 3600, delay: 500 },
  { x: 84, y: 18, s: 2, d: 4600, delay: 1400 },
  { x: 30, y: 58, s: 2, d: 3900, delay: 2100 },
];

function Star({ x, y, s, d, delay, animate }: (typeof STARS)[number] & { animate: boolean }) {
  const a = useRef(new Animated.Value(0.45)).current;
  useEffect(() => {
    if (!animate) return;
    const loop = Animated.loop(
      Animated.sequence([
        Animated.delay(delay),
        Animated.timing(a, { toValue: 1, duration: d / 2, useNativeDriver: false }),
        Animated.timing(a, { toValue: 0.25, duration: d / 2, useNativeDriver: false }),
      ]),
    );
    loop.start();
    return () => loop.stop();
  }, [a, animate, d, delay]);
  return (
    <Animated.View
      style={[styles.star, { width: s, height: s, transform: [{ translateX: x }, { translateY: y }], opacity: a }]}
    />
  );
}

/**
 * The poster screens' mark: tonight's real moon over a soft warm glow, with a
 * few stars that breathe. Motion stops when the phone asks for reduced motion.
 */
export function MoonMark() {
  const { f, waxing } = lunarPhase();
  const reduce = useReduceMotion();
  return (
    <View style={styles.wrap} accessibilityLabel="Yume">
      <View style={[styles.glow, styles.glowOuter]} />
      <View style={[styles.glow, styles.glowInner]} />
      {STARS.map((st, i) => (
        <Star key={i} {...st} animate={!reduce} />
      ))}
      <View style={waxing ? undefined : styles.mirror}>
        <MoonProgress f={Math.max(0.06, f)} size={64} color={colors.moon} />
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: {
    alignItems: 'center',
    justifyContent: 'center',
    marginBottom: spacing.md,
    height: 140,
  },
  mirror: {
    transform: [{ scaleX: -1 }],
  },
  glow: {
    position: 'absolute',
    borderRadius: radius.pill,
    backgroundColor: colors.moon,
  },
  glowOuter: {
    width: 150,
    height: 150,
    opacity: 0.04,
  },
  glowInner: {
    width: 96,
    height: 96,
    opacity: 0.07,
  },
  star: {
    position: 'absolute',
    borderRadius: radius.pill,
    backgroundColor: colors.moon,
  },
});

