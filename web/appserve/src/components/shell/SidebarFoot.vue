<script lang="ts">
import { ref } from 'vue';

/** Whether the theme menu is open: drawn outside the sidebar, it keeps the peek open (SidebarPeek). */
export const themeMenuOpen = ref(false);
</script>

<script setup lang="ts">
// The sidebar's foot, as Cloud draws it: whether anfra answers (a dot, with its meaning also in
// text), the Alpha badge of the alpha launch (ANF-82; the neutral badge, so it does not read as a
// status), a newer release when there is one, and the theme menu (Light, Dark, System).
import { Monitor, Moon, Sun } from '@lucide/vue';
import { computed, onBeforeUnmount } from 'vue';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { StatusPill } from '@/components/ui/status-pill';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import type * as server from '@/lib/server';
import type { ThemeChoice } from '@/lib/theme';

const props = defineProps<{
  health: server.Health | 'unknown';
  version?: server.Version;
  themeChoice: ThemeChoice;
  /** The theme in effect, which the menu's button shows. */
  theme: 'light' | 'dark';
}>();

const emit = defineEmits<{ setTheme: [choice: ThemeChoice] }>();

// A release version reads as v0.4.2; a local build's ("dev") as itself.
const versionLabel = computed(() => {
  const current = props.version?.current;
  if (!current) return undefined;
  return /^\d/.test(current) ? `v${current}` : current;
});

const statusLabel = computed(() => {
  if (props.health === 'unreachable') return 'anfra server unreachable';
  if (props.health === 'up') return versionLabel.value ? `anfra ${versionLabel.value}` : 'anfra is running';
  if (props.health === 'down') return 'anfra is down';
  return 'Checking anfra…';
});

const dot = computed(() => {
  if (props.health === 'up') return 'bg-success-default';
  if (props.health === 'unknown') return 'bg-skeleton';
  return 'bg-danger-default';
});

const updateHint = computed(() => (props.version?.latest
  ? `anfra ${props.version.latest} is available (you have ${props.version.current}). Run \`anfra update\`, then restart anfra serve.`
  : ''));

// This foot's menu, open: one unmounted open (the peek closing under it) never says it closed.
const menuOpen = ref(false);
function onMenuOpen (open: boolean): void {
  menuOpen.value = open;
  themeMenuOpen.value = open;
}
onBeforeUnmount(() => { if (menuOpen.value) themeMenuOpen.value = false; });

const THEMES: { value: ThemeChoice, label: string, icon: typeof Sun }[] = [
  { value: 'light', label: 'Light', icon: Sun },
  { value: 'dark', label: 'Dark', icon: Moon },
  { value: 'system', label: 'System', icon: Monitor },
];
</script>

<template>
  <div class="shrink-0 p-2">
    <div class="flex items-center gap-2 border-t border-sidebar-border pt-2 pl-2">
      <span
        class="flex min-w-0 flex-1 items-center gap-2"
        role="status"
        data-testid="anfra-status"
        :data-status="health"
        :title="health === 'up' && version ? `anfra is running on version ${version.current}` : statusLabel"
      >
        <span aria-hidden="true" class="size-2 shrink-0 rounded-pill" :class="dot" data-testid="status-dot" />
        <span class="truncate text-caption text-weak">{{ statusLabel }}</span>
        <Badge variant="secondary" data-testid="alpha-badge">Alpha</Badge>
        <StatusPill
          v-if="version?.latest && health !== 'unreachable'"
          class="cursor-help"
          tone="highlight"
          data-testid="update-badge"
          role="note"
          :aria-label="updateHint"
          :title="updateHint"
        >
          Update
        </StatusPill>
      </span>
      <Tooltip>
        <DropdownMenu @update:open="onMenuOpen">
          <DropdownMenuTrigger as-child>
            <TooltipTrigger as-child>
              <Button variant="ghost" size="icon" aria-label="Theme" data-testid="theme-menu">
                <Moon v-if="theme === 'dark'" />
                <Sun v-else />
              </Button>
            </TooltipTrigger>
          </DropdownMenuTrigger>
          <DropdownMenuContent side="top" align="end" :side-offset="6" class="min-w-36" data-testid="theme-options">
            <DropdownMenuRadioGroup
              :model-value="themeChoice"
              @update:model-value="(choice) => emit('setTheme', choice as ThemeChoice)"
            >
              <DropdownMenuRadioItem v-for="option in THEMES" :key="option.value" :value="option.value" :data-theme-option="option.value">
                <component :is="option.icon" class="icon-weak" />
                {{ option.label }}
              </DropdownMenuRadioItem>
            </DropdownMenuRadioGroup>
          </DropdownMenuContent>
        </DropdownMenu>
        <TooltipContent side="top">Theme</TooltipContent>
      </Tooltip>
    </div>
  </div>
</template>
