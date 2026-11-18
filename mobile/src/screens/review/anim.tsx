import { useEffect, useRef, useState, type ReactNode } from 'react';
import { Animated, Platform, type StyleProp, type ViewStyle } from 'react-native';

// Web-safe driver pick: react-native-web only supports the JS driver.
export const NATIVE_DRIVER = Platform.OS !== 'web';

/** Fade (+ optional upward drift) on mount — transform/opacity only, so it can
 *  never reflow settled layout. Re-key to replay (the card advance does). */
export function FadeIn({
  children,
  style,
  duration = 150,
  drift = 0,
}: {
  children: ReactNode;
  style?: StyleProp<ViewStyle>;
  duration?: number;
  drift?: number;
}) {
  const anim = useRef(new Animated.Value(0)).current;
  useEffect(() => {
    Animated.timing(anim, { toValue: 1, duration, useNativeDriver: NATIVE_DRIVER }).start();
  }, [anim, duration]);
  return (
    <Animated.View
      style={[
        style,
        { opacity: anim },
        drift
          ? { transform: [{ translateY: anim.interpolate({ inputRange: [0, 1], outputRange: [drift, 0] }) }] }
          : null,
      ]}
    >
      {children}
    </Animated.View>
  );
}

/** Visibility-driven fade that stays mounted through the fade-out (callers
 *  latch any content derived from the state that just went null). */
export function Fade({
  visible,
  children,
  style,
  pointerEvents,
}: {
  visible: boolean;
  children: ReactNode;
  style?: StyleProp<ViewStyle>;
  pointerEvents?: 'none' | 'auto' | 'box-none';
}) {
  const anim = useRef(new Animated.Value(visible ? 1 : 0)).current;
  const [mounted, setMounted] = useState(visible);
  useEffect(() => {
    if (visible) setMounted(true);
    Animated.timing(anim, { toValue: visible ? 1 : 0, duration: 150, useNativeDriver: NATIVE_DRIVER })
      .start(({ finished }) => { if (finished && !visible) setMounted(false); });
  }, [anim, visible]);
  if (!visible && !mounted) return null;
  return (
    <Animated.View
      style={[style, { opacity: anim }]}
      pointerEvents={pointerEvents ?? (visible ? 'auto' : 'none')}
    >
      {children}
    </Animated.View>
  );
}
