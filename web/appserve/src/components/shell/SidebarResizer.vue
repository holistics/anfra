<script setup lang="ts">
// The sidebar's edge, as Cloud's: drag it to resize, click it to collapse (so does the shortcut). A
// violet line, faded to nothing at its ends, shows under the pointer and through a drag, so the
// edge reads as a handle; a tooltip names both actions and the shortcut. The shell follows the
// width as it is dragged, keeps the Data App's frame from taking the pointer from the press to
// the release (a first move that lands on the frame would go into it before the capture holds),
// and remembers the width once the drag ends.
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { clampWidth, DRAG_PX, SIDEBAR_SHORTCUT } from '@/lib/sidebar';
import ShortcutKeys from './ShortcutKeys.vue';

const props = defineProps<{ width: number }>();

const emit = defineEmits<{
  /** The edge is pressed: until `release`, no frame takes the pointer. */
  press: [];
  /** Dragged to this width. */
  resize: [width: number];
  /** Let go, at the width dragged to; without one it was a click. */
  release: [width?: number];
  collapse: [];
}>();

let dragged = false;

function down (event: PointerEvent): void {
  dragged = false;
  if (event.button !== 0) return;
  event.preventDefault();
  const rail = event.currentTarget as HTMLElement;
  rail.setPointerCapture(event.pointerId);
  emit('press');
  const startX = event.clientX;
  const startWidth = props.width;
  let width = startWidth;

  const move = (moved: PointerEvent): void => {
    if (!dragged && Math.abs(moved.clientX - startX) < DRAG_PX) return;
    dragged = true;
    width = clampWidth(startWidth + moved.clientX - startX);
    emit('resize', width);
  };
  const up = (): void => {
    rail.removeEventListener('pointermove', move);
    rail.removeEventListener('pointerup', up);
    rail.removeEventListener('pointercancel', up);
    emit('release', dragged ? width : undefined);
  };
  rail.addEventListener('pointermove', move);
  rail.addEventListener('pointerup', up);
  rail.addEventListener('pointercancel', up);
}

/** A click that ends a drag does not also collapse the sidebar. */
function click (): void {
  if (!dragged) emit('collapse');
}
</script>

<template>
  <Tooltip disable-hoverable-content>
    <TooltipTrigger as-child>
      <!-- The line is the button's ::after: a gradient from nothing, through the primary, to nothing. -->
      <button
        type="button"
        aria-label="Resize or collapse the sidebar"
        :tabindex="-1"
        class="fixed inset-y-0 z-20 w-4 -translate-x-1/2 cursor-col-resize touch-none after:absolute after:inset-y-0 after:start-1/2 after:w-0.5 after:-translate-x-1/2 after:bg-linear-to-b after:from-transparent after:via-primary after:to-transparent after:opacity-0 after:transition-opacity after:duration-160 after:ease-anfra-out hover:after:opacity-100 in-data-[pressing]:after:opacity-100"
        :style="{ left: `${width}px` }"
        data-testid="sidebar-rail"
        @pointerdown="down"
        @click="click"
      />
    </TooltipTrigger>
    <TooltipContent side="right" :side-offset="-9">
      Drag to resize · Click to collapse
      <ShortcutKeys :combo="SIDEBAR_SHORTCUT" />
    </TooltipContent>
  </Tooltip>
</template>
