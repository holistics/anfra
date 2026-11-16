<script setup lang="ts">
// The sidebar's foot, as Cloud draws it: whether anfra answers (a dot, with its meaning also in
// text), the Alpha badge of the alpha launch (ANF-82; the neutral badge, so it does not read as a
// status), a newer release when there is one, and the theme menu (Light, Dark, System).
// In the peek, the menu is drawn inside the peek's panel and is not modal: pointing at it or
// focusing it is pointing at or focusing the panel, so the peek stays open with no bookkeeping.
import { Monitor, Moon, Sun } from '@lucide/vue';
import { computed, ref, watch } from 'vue';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { StatusPill } from '@/components/ui/status-pill';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import type * as server from '@/lib/server';
import type { ThemeChoice } from '@/lib/theme';

const props = withDefaults(defineProps<{
  health: server.Health | 'unknown';
  version?: server.Version;
  themeChoice: ThemeChoice;
  /** The theme in effect, which the menu's button shows. */
  theme: 'light' | 'dark';
  /** In the sidebar's peek: the menu is drawn inside its panel. */
  inPeek?: boolean;
  /** Whether the sidebar holding this foot shows: when it collapses, the menu closes with it. */
  shown?: boolean;
}>(), { shown: true });

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

const menuOpen = ref(false);
// A menu left open on a collapsed sidebar would float by nothing, and keep Escape from the shell.
watch(() => props.shown, (shown) => { if (shown === false) menuOpen.value = false; });

const THEMES: { value: ThemeChoice, label: string, icon: typeof Sun }[] = [
  { value: 'light', label: 'Light', icon: Sun },
  { value: 'dark', label: 'Dark', icon: Moon },
  { value: 'system', label: 'System', icon: Monitor },
];
</script>

<template>
  <div class="shrink-0 p-2">
    <div class="flex items-center gap-2 border-t border-sidebar-border pt-2 pl-2">
      <span class="flex min-w-0 flex-1 items-center gap-2">
        <!-- The live region is the status alone, so a change of health doesn't also read out the badges. -->
        <span
          class="flex min-w-0 items-center gap-2"
          role="status"
          data-testid="anfra-status"
          :data-status="health"
          :title="health === 'up' && version ? `anfra is running on version ${version.current}` : statusLabel"
        >
          <span aria-hidden="true" class="size-2 shrink-0 rounded-pill" :class="dot" data-testid="status-dot" />
          <span class="truncate text-caption text-weak">{{ statusLabel }}</span>
        </span>
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
        <DropdownMenu v-model:open="menuOpen" :modal="!inPeek">
          <DropdownMenuTrigger as-child>
            <TooltipTrigger as-child>
              <Button variant="ghost" size="icon" aria-label="Theme" data-testid="theme-menu">
                <Moon v-if="theme === 'dark'" />
                <Sun v-else />
              </Button>
            </TooltipTrigger>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            side="top"
            align="end"
            :side-offset="6"
            :portal="!inPeek"
            class="min-w-36"
            data-testid="theme-options"
          >
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
