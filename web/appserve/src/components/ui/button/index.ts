import type { VariantProps } from 'class-variance-authority';
import { cva } from 'class-variance-authority';

export { default as Button } from './Button.vue';

export const buttonVariants = cva(
  'group/button inline-flex shrink-0 items-center justify-center rounded-lg border border-transparent bg-clip-padding text-label whitespace-nowrap transition-[background-color,border-color,color,scale] duration-160 ease-anfra-out outline-none select-none active:duration-90 active:not-aria-[haspopup]:scale-97 disabled:pointer-events-none disabled:bg-disabled disabled:text-disabled aria-invalid:border-danger-strong [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*=size-])]:size-4',
  {
    variants: {
      variant: {
        default:
          'bg-primary text-primary-foreground hover:bg-highlight-hover active:bg-highlight-active disabled:border-disabled',
        outline:
          'border-strong bg-default text-default hover:border-hover active:bg-active disabled:border-disabled aria-expanded:bg-active aria-pressed:bg-active',
        secondary:
          'bg-fill-default text-secondary-foreground hover:bg-fill-hover disabled:border-disabled aria-expanded:bg-active',
        ghost:
          'text-weak hover:bg-hover hover:text-default active:bg-active disabled:bg-transparent aria-expanded:bg-active aria-expanded:text-default aria-pressed:bg-active aria-pressed:text-default',
        destructive:
          'bg-danger-default text-on-fill hover:bg-danger-hover active:bg-danger-hover disabled:border-disabled',
        link: 'text-link-default underline-offset-4 hover:text-link-hover hover:underline disabled:bg-transparent',
      },
      size: {
        default:
          'h-8 gap-1.5 px-2.5 has-data-[icon=inline-end]:pr-2 has-data-[icon=inline-start]:pl-2',
        xs: 'h-6 gap-1 rounded-md px-2 text-caption font-medium in-data-[slot=button-group]:rounded-md has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 [&_svg:not([class*=size-])]:size-3',
        sm: 'h-7 gap-1 rounded-md px-2.5 text-caption font-medium in-data-[slot=button-group]:rounded-md has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 [&_svg:not([class*=size-])]:size-3.5',
        lg: 'h-9 gap-1.5 px-2.5 has-data-[icon=inline-end]:pr-2 has-data-[icon=inline-start]:pl-2',
        icon: 'size-8',
        'icon-xs':
          'size-6 rounded-md in-data-[slot=button-group]:rounded-md [&_svg:not([class*=size-])]:size-3',
        'icon-sm': 'size-7 rounded-md in-data-[slot=button-group]:rounded-md',
        'icon-lg': 'size-9',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
);
export type ButtonVariants = VariantProps<typeof buttonVariants>;
