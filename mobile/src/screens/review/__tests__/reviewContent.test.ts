jest.mock('@react-native-async-storage/async-storage', () =>
  require('@react-native-async-storage/async-storage/jest/async-storage-mock'),
);

import type { YumeClient } from '../../../api/client';
import type { CardMediaInfo, MediaBlob } from '../../../api/types';
import {
  clearAccountContent,
  loadCardImage,
  peekAudioBytes,
  peekPrimaryMedia,
  prefetchCardAudio,
} from '../reviewContent';
import { selectAudioClips } from '../reviewMedia';

function media(
  id: number,
  kind: CardMediaInfo['kind'],
  label?: string,
): CardMediaInfo {
  return {
    id,
    card_id: 7,
    kind,
    label,
    file: `${id}.wav`,
    is_primary: false,
    created_at: 0,
  };
}

describe('selectAudioClips', () => {
  it('orders word audio before sentence audio', () => {
    expect(
      selectAudioClips([
        media(3, 'audio', 'sentence'),
        media(2, 'image'),
        media(1, 'audio', 'word'),
      ]).map((item) => item.id),
    ).toEqual([1, 3]);
  });

  it('drops superseded unlabeled audio when labeled clips exist', () => {
    expect(
      selectAudioClips([
        media(1, 'audio'),
        media(2, 'audio', 'word'),
        media(3, 'audio', 'sentence'),
      ]).map((item) => item.id),
    ).toEqual([2, 3]);
  });

  it('keeps legacy unlabeled audio when it is all the card has', () => {
    expect(selectAudioClips([media(2, 'audio'), media(1, 'audio')]).map((item) => item.id))
      .toEqual([1, 2]);
  });
});

describe('review image cache', () => {
  beforeEach(() => clearAccountContent());

  it('fetches a card image once and reuses it during the session', async () => {
    const image: MediaBlob = { mime: 'image/jpeg', data: 'base64-image' };
    const getCardMedia = jest.fn().mockResolvedValue({ image });
    const client = { getCardMedia } as unknown as YumeClient;

    await expect(loadCardImage(client, 7, () => true)).resolves.toEqual(image);
    await expect(loadCardImage(client, 7, () => true)).resolves.toEqual(image);
    expect(getCardMedia).toHaveBeenCalledTimes(1);
  });

  it('does not cache media returned after an account switch', async () => {
    const image: MediaBlob = { mime: 'image/png', data: 'old-account' };
    const getCardMedia = jest.fn().mockResolvedValue({ image });
    const client = { getCardMedia } as unknown as YumeClient;

    await expect(loadCardImage(client, 7, () => false)).resolves.toBeNull();
    await expect(loadCardImage(client, 7, () => true)).resolves.toEqual(image);
    expect(getCardMedia).toHaveBeenCalledTimes(2);
  });
});

describe('card audio prefetch', () => {
  beforeEach(() => clearAccountContent());

  it('fetches the clip list, every clip and the primary clip before the tap', async () => {
    const primary = { audio: { mime: 'audio/wav', data: 'primary' } };
    const client = {
      getCardMedia: jest.fn().mockResolvedValue(primary),
      listCardMedia: jest.fn().mockResolvedValue([media(1, 'audio', 'word'), media(2, 'audio', 'sentence')]),
      getCardMediaFile: jest.fn((id: number) => Promise.resolve({ mime: 'audio/wav', data: `clip-${id}` })),
    };

    const clips = await prefetchCardAudio(client as unknown as YumeClient, 7, () => true);
    expect(clips.map((clip) => clip.id)).toEqual([1, 2]);
    await new Promise((resolve) => setTimeout(resolve, 0));
    // A tap can now play synchronously from cache.
    expect(peekPrimaryMedia(7)).toEqual(primary);
    expect(peekAudioBytes(1)?.data).toBe('clip-1');
    expect(peekAudioBytes(2)?.data).toBe('clip-2');
  });

  it('shares one request between the audio prefetch and the image load', async () => {
    const getCardMedia = jest.fn().mockResolvedValue({ image: { mime: 'image/png', data: 'img' } });
    const client = {
      getCardMedia,
      listCardMedia: jest.fn().mockResolvedValue([]),
      getCardMediaFile: jest.fn(),
    } as unknown as YumeClient;

    await Promise.all([prefetchCardAudio(client, 7, () => true), loadCardImage(client, 7, () => true)]);
    expect(getCardMedia).toHaveBeenCalledTimes(1);
  });
});
