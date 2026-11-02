import type { VariantProps } from 'class-variance-authority';
import { cva } from 'class-variance-authority';

export { default as Badge } from './Badge.vue';

export const badgeVariants = cva(
  'group/badge inline-flex h-5 w-fit shrink-0 items-center justify-center gap-1 overflow-hidden rounded-pill border border-transparent px-2 py-0.5 text-caption font-medium whitespace-nowrap transition-colors duration-160 ease-anfra-out has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 aria-invalid:border-danger-strong [&>svg]:pointer-events-none [&>svg]:size-3!',
  {
    variants: {
      variant: {
        default: 'bg-primary text-primary-foreground [a]:hover:bg-highlight-hover',
        secondary: 'border-default bg-info-secondary-default text-info-default [a]:hover:bg-active',
        destructive: 'border-danger-default bg-danger-secondary-default text-danger-default',
        outline: 'border-strong text-default [a]:hover:bg-hover',
        ghost: 'text-weak hover:bg-hover hover:text-default',
        link: 'text-link-default underline-offset-4 hover:text-link-hover hover:underline',
        warning: 'border-warning-default bg-warning-secondary-default text-warning-default',
        success: 'border-success-default bg-success-secondary-default text-success-default',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
);
export type BadgeVariants = VariantProps<typeof badgeVariants>;
