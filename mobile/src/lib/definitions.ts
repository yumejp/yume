import type { DictEntry } from '../api/types';
import type { DefinitionLang } from '../settings/SettingsContext';

/** Definition language. Entries carry the language of their glosses since
 *  schema 21; an older server marks imported dictionaries only by a negative
 *  id, which is read as Japanese. */
export function isJapaneseEntry(e: DictEntry): boolean {
  return e.lang ? e.lang === 'ja' : e.id < 0;
}

/** Entries in the order the learner asked for: Japanese definitions first
 *  when the setting is Japanese, otherwise as the server ranked them
 *  (JMdict first, imported dictionaries after). */
export function orderEntries<T extends DictEntry>(entries: T[], lang: DefinitionLang): T[] {
  if (lang !== 'ja') return entries;
  return [...entries.filter(isJapaneseEntry), ...entries.filter((e) => !isJapaneseEntry(e))];
}

/** The Japanese definition to show in place of the English glosses, or null
 *  when the dictionary has none for this word. */
export function japaneseDefinition(entries: DictEntry[]): string | null {
  const e = entries.find(isJapaneseEntry);
  const glosses = e?.senses?.flatMap((s) => s.glosses ?? []) ?? [];
  return glosses.length ? glosses.join(' ') : null;
}
