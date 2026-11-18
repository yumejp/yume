import { Platform } from 'react-native';

/** Availability of the supported Japanese text-input modes. */
export type MeetingCapability = 'native' | 'dev' | 'none';

/** iOS major version, or 0 when not on iOS. Platform.Version is a string
 *  like "26.0" on iOS and an API-level number on Android. */
function iosMajor(): number {
  if (Platform.OS !== 'ios') return 0;
  const v = String(Platform.Version).split('.')[0];
  return Number(v) || 0;
}

/** The native transcriber module, when the dev-build shell provides it.
 *  Optional require so Expo Go / web bundles don't crash on a module that
 *  isn't linked — Metro resolves it lazily and we swallow the miss. */
export function nativeTranscriber(): unknown | null {
  if (Platform.OS !== 'ios') return null;
  try {
    // eslint-disable-next-line @typescript-eslint/no-var-requires
    const mod = require('../../modules/yume-transcriber');
    return mod?.default ?? mod ?? null;
  } catch {
    return null;
  }
}

export function meetingCapability(devIdentity: boolean): MeetingCapability {
  if (Platform.OS === 'ios' && iosMajor() >= 26 && nativeTranscriber() != null) {
    return 'native';
  }
  return devIdentity ? 'dev' : 'none';
}
