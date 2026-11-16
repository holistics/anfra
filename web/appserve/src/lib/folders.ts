// Which of the tree's folders show collapsed. Folders start open; the reader collapses what they
// don't need, and that is kept on this device (through vueuse's useStorage, as the theme and the
// sidebar are). Opening a Data App shows the folders it sits in, and so does a breadcrumb, but
// only for this visit: a link or a reload never changes what the reader chose to keep collapsed.
import { useStorage } from '@vueuse/core';
import { computed, ref } from 'vue';

export const FOLDERS_KEY = 'anfra-appserve:collapsed-folders';

export function useFolders () {
  const kept = useStorage(FOLDERS_KEY, new Set<string>(), undefined, { writeDefaults: false, onError: () => {} });
  // The kept-collapsed folders shown open for this visit.
  const revealed = ref(new Set<string>());

  /** The folders that show collapsed. */
  const collapsed = computed(() => new Set([...kept.value].filter((folder) => !revealed.value.has(folder))));

  /** The reader's own toggle, of the folder as it shows: it is what's kept. */
  function toggle (path: string): void {
    const next = new Set(kept.value);
    if (collapsed.value.has(path)) next.delete(path);
    else next.add(path);
    if (revealed.value.has(path)) revealed.value = new Set([...revealed.value].filter((folder) => folder !== path));
    kept.value = next;
  }

  /** Shows these folders open, for this visit. */
  function reveal (paths: string[]): void {
    const closed = paths.filter((folder) => collapsed.value.has(folder));
    if (closed.length) revealed.value = new Set([...revealed.value, ...closed]);
  }

  return { collapsed, toggle, reveal };
}
