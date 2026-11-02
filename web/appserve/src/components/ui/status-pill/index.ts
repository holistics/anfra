import type { VariantProps } from 'class-variance-authority';
import { cva } from 'class-variance-authority';

export { default as StatusPill } from './StatusPill.vue';

// The state of a thing, as a small pill with a dot: violet for what the reader
// follows (published, verified), neutral for inactive, and the restrained status
// hues. Only these tones exist, so no screen can add a new status color. A pill
// always carries a text label: the color is never the only cue (WCAG 1.4.1).
export const statusPillVariants = cva(
  'inline-flex h-5 w-fit shrink-0 items-center gap-1.5 rounded-pill px-2 text-caption font-medium whitespace-nowrap before:size-1.5 before:shrink-0 before:rounded-full before:bg-current',
  {
    variants: {
      tone: {
        highlight: 'bg-highlight-secondary-default text-highlight-default',
        neutral: 'bg-active text-weak',
        success: 'bg-success-secondary-default text-success-default',
        warning: 'bg-warning-secondary-default text-warning-default',
        danger: 'bg-danger-secondary-default text-danger-default',
      },
    },
    defaultVariants: {
      tone: 'neutral',
    },
  },
);
export type StatusPillVariants = VariantProps<typeof statusPillVariants>;
