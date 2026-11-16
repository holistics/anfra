<script setup lang="ts">
// The sidebar's toggle in the header, and, while the sidebar is collapsed on a wide window, its
// peek, as Cloud has them: pointing at the toggle or focusing it floats the sidebar's content
// (SidebarPanel, the same component) below it. The peek closes 150 ms after the pointer leaves the
// toggle and the panel, when focus leaves both with the pointer away, on Escape, and when the
// sidebar opens. The theme menu is drawn inside the panel (SidebarFoot), so using it is using the
// panel. Clicking the toggle opens the sidebar, which is the keyboard path to the content too (so
// is the shortcut). The tooltip names the shortcut and sits right of the toggle, so it shows with
// the peek and never covers the panel below. The content is the slot, so the peek shows what the
// sidebar does.
import { PanelLeft } from '@lucide/vue';
import { ref, watch } from 'vue';
import { Button } from '@/components/ui/button';
import { Popover, PopoverAnchor, PopoverContent } from '@/components/ui/popover';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { SIDEBAR_SHORTCUT } from '@/lib/sidebar';
import ShortcutKeys from './ShortcutKeys.vue';

const props = defineProps<{ open: boolean, narrow: boolean, width: number }>();
const emit = defineEmits<{ toggleSidebar: [] }>();

const CLOSE_DELAY_MS = 150;

const peeking = ref(false);
/** Whether the pointer is on the toggle or the panel. */
const over = ref(false);
let timer: ReturnType<typeof setTimeout> | undefined;

const anchor = ref<HTMLElement | null>(null);
// The panel's content: PopoverContent's own root is its portal's placeholder, which holds nothing.
const panel = ref<HTMLElement | null>(null);
const within = (node: EventTarget | null): boolean => node instanceof Node
  && !!(anchor.value?.contains(node) || panel.value?.contains(node));

function peek (): void {
  clearTimeout(timer);
  if (!props.open && !props.narrow) peeking.value = true;
}
function hideSoon (): void {
  clearTimeout(timer);
  timer = setTimeout(() => { peeking.value = false; }, CLOSE_DELAY_MS);
}
function hideNow (): void {
  clearTimeout(timer);
  peeking.value = false;
}
function pointerEnters (): void {
  over.value = true;
  peek();
}
function pointerLeaves (): void {
  over.value = false;
  hideSoon();
}
/** Focus coming to the toggle or the panel from elsewhere peeks; focus moving between them changes nothing. */
function focusEnters (event: FocusEvent): void {
  if (!within(event.relatedTarget)) peek();
}
/** Focus leaving both closes the peek, unless the pointer is still on it (a click on its padding takes focus away). */
function focusLeaves (event: FocusEvent): void {
  if (!within(event.relatedTarget) && !over.value) hideSoon();
}

watch(() => props.open, (open) => { if (open) hideNow(); });
watch(() => props.narrow, (narrow) => { if (narrow) hideNow(); });
</script>

<template>
  <Popover :open="peeking" @update:open="(open: boolean) => !open && hideNow()">
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
              @pointerenter="pointerEnters"
              @focus="focusEnters"
              @blur="focusLeaves"
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
    <!-- Fades in and scales from 0.97 at its corner under the toggle, as floating surfaces do; out, a quicker fade. -->
    <PopoverContent
      side="bottom"
      align="start"
      :side-offset="4"
      :collision-padding="{ top: 8, right: 8, bottom: 16, left: 8 }"
      data-testid="sidebar-peek"
      aria-label="Sidebar"
      class="h-(--reka-popover-content-available-height) max-w-none origin-(--reka-popover-content-transform-origin) gap-0 bg-sidebar p-2 text-sidebar-foreground ring-1 ring-weak ease-(--anfra-ease-out) data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:duration-(--anfra-t-press) data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-97 data-[state=open]:duration-(--anfra-t-hover)"
      :style="{ width: `${width}px` }"
      @open-auto-focus.prevent
      @close-auto-focus.prevent
      @interact-outside="(event: Event) => within(event.target) && event.preventDefault()"
      @pointerenter="pointerEnters"
      @pointermove="pointerEnters"
      @pointerleave="pointerLeaves"
      @focusin="focusEnters"
      @focusout="focusLeaves"
    >
      <div ref="panel" class="flex min-h-0 flex-1 flex-col">
        <slot />
      </div>
    </PopoverContent>
  </Popover>
</template>
