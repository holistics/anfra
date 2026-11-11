<script setup lang="ts">
// The sidebar's toggle in the header, and, while the sidebar is collapsed on a wide window, its
// peek, as Cloud has them: pointing at the toggle or focusing it floats the sidebar's content
// (SidebarPanel, the same component) below it. The peek closes 150 ms after the pointer leaves the
// toggle and the panel, on Escape, when focus leaves both, and when the sidebar opens; it stays
// while the theme menu, drawn outside it, is open. Clicking the toggle opens the sidebar, which is
// the keyboard path to the content too (so is the shortcut). The tooltip names the shortcut and
// sits right of the toggle, so it shows with the peek and never covers the panel below. The
// content is the slot, so the peek shows what the sidebar does.
import { PanelLeft } from '@lucide/vue';
import { ref, watch } from 'vue';
import { Button } from '@/components/ui/button';
import { Popover, PopoverAnchor, PopoverContent } from '@/components/ui/popover';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { SIDEBAR_SHORTCUT } from '@/lib/sidebar';
import ShortcutKeys from './ShortcutKeys.vue';
import { themeMenuOpen } from './SidebarFoot.vue';

const props = defineProps<{ open: boolean, narrow: boolean, width: number }>();
const emit = defineEmits<{ toggleSidebar: [] }>();

const CLOSE_DELAY_MS = 150;
const MENU_FOCUS_MS = 300;

const peeking = ref(false);
/** Whether the pointer is on the toggle or the panel. */
const over = ref(false);
let timer: ReturnType<typeof setTimeout> | undefined;

function show (): void {
  over.value = true;
  clearTimeout(timer);
  if (!props.open && !props.narrow) peeking.value = true;
}
/** The theme menu is drawn outside the panel: while it is open the peek stays, and closes after it if the pointer is away. */
function hideSoon (): void {
  clearTimeout(timer);
  timer = setTimeout(() => {
    if (!themeMenuOpen.value) peeking.value = false;
  }, CLOSE_DELAY_MS);
}
/** When a menu from the panel last closed: it gives focus back to its button, which is not the reader's return. */
let menuClosedAt = 0;
function focusIn (): void {
  if (performance.now() - menuClosedAt < MENU_FOCUS_MS) return;
  show();
}
function pointerLeaves (): void {
  over.value = false;
  hideSoon();
}
function hideNow (): void {
  clearTimeout(timer);
  peeking.value = false;
}
/** Focus moving between the toggle and the panel keeps the peek; moving out of both closes it. */
function leaves (event: FocusEvent, container: HTMLElement | null | undefined): void {
  const next = event.relatedTarget as Node | null;
  if (next && container?.contains(next)) return;
  hideSoon();
}

watch(() => props.open, (open) => { if (open) hideNow(); });
watch(() => props.narrow, (narrow) => { if (narrow) hideNow(); });
watch(themeMenuOpen, (open) => {
  if (open) return;
  menuClosedAt = performance.now();
  if (!over.value) hideSoon();
});

const anchor = ref<HTMLElement | null>(null);
// The panel's content: PopoverContent's own root is its portal's placeholder, which holds nothing.
const panel = ref<HTMLElement | null>(null);
</script>

<template>
  <Popover :open="peeking" @update:open="(open: boolean) => !open && !themeMenuOpen && hideNow()">
    <PopoverAnchor as-child>
      <span ref="anchor" class="inline-flex" @pointerleave="pointerLeaves">
        <Tooltip disable-hoverable-content>
          <TooltipTrigger as-child>
            <Button
              variant="ghost"
              size="icon-sm"
              :aria-label="open ? 'Hide sidebar' : 'Show sidebar'"
              :aria-keyshortcuts="SIDEBAR_SHORTCUT.replace('Ctrl', 'Control')"
              data-testid="sidebar-toggle"
              @click="emit('toggleSidebar')"
              @pointerenter="show"
              @focus="show"
              @blur="(event: FocusEvent) => leaves(event, panel)"
            >
              <PanelLeft />
            </Button>
          </TooltipTrigger>
          <TooltipContent side="right">
            Toggle sidebar
            <ShortcutKeys :combo="SIDEBAR_SHORTCUT" />
          </TooltipContent>
        </Tooltip>
      </span>
    </PopoverAnchor>
    <PopoverContent
      side="bottom"
      align="start"
      :side-offset="4"
      :collision-padding="{ top: 8, right: 8, bottom: 16, left: 8 }"
      data-peek
      data-testid="sidebar-peek"
      aria-label="Sidebar"
      class="h-(--reka-popover-content-available-height) max-w-none gap-0 bg-sidebar p-2 text-sidebar-foreground ring-1 ring-weak"
      :style="{ width: `${width}px` }"
      @open-auto-focus.prevent
      @close-auto-focus.prevent
      @interact-outside="(event: Event) => themeMenuOpen && event.preventDefault()"
      @pointerenter="show"
      @pointermove="show"
      @pointerleave="pointerLeaves"
      @focusin="focusIn"
      @focusout="(event: FocusEvent) => leaves(event, anchor)"
    >
      <div ref="panel" class="flex min-h-0 flex-1 flex-col">
        <slot />
      </div>
    </PopoverContent>
  </Popover>
</template>
