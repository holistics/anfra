import type { ClassValue } from 'clsx';
import { clsx } from 'clsx';
import { extendTailwindMerge } from 'tailwind-merge';

/** The type roles in main.css (`--text-*`): without them, twMerge reads `text-title` as a color and drops it beside `text-weak`. */
const TYPE_ROLES = ['title', 'heading', 'body', 'label', 'caption', 'code'];

const twMerge = extendTailwindMerge({
  extend: { theme: { text: TYPE_ROLES } },
});

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
