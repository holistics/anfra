// Tooltips: what a control is and its shortcut, shown on hover after a beat (at once while moving
// from one tooltip to the next) and on keyboard focus. One tooltip shows at a time; <Tooltip />,
// mounted once in the app, draws it. Put one on any element with `v-tip="'Undo'"` or
// `v-tip="{ text: 'Undo', keys: '⌘Z' }"`; Button takes `label` and `keys` and does it for you.
import { reactive, type Directive } from 'vue';

export interface Tip {
  text: string;
  keys?: string;
}

export const tip = reactive({ text: '', keys: '', x: 0, y: 0, above: false, open: false });

const DELAY = 500;
const GRACE = 400;
let timer: ReturnType<typeof setTimeout> | undefined;
let owner: HTMLElement | null = null;
let hiddenAt = 0;

function show (el: HTMLElement, t: Tip, now = false) {
  clearTimeout(timer);
  owner = el;
  const open = () => {
    if (owner !== el || !el.isConnected) return;
    const r = el.getBoundingClientRect();
    tip.text = t.text;
    tip.keys = t.keys ?? '';
    tip.x = r.left + r.width / 2;
    tip.above = r.top > window.innerHeight / 2;
    tip.y = tip.above ? r.top - 8 : r.bottom + 8;
    tip.open = true;
  };
  if (now || tip.open || Date.now() - hiddenAt < GRACE) open();
  else timer = setTimeout(open, DELAY);
}

export function hideTip (el?: HTMLElement) {
  if (el && owner !== el) return;
  clearTimeout(timer);
  owner = null;
  if (tip.open) hiddenAt = Date.now();
  tip.open = false;
}

const tips = new WeakMap<HTMLElement, { tip: Tip | undefined, off: () => void }>();
const norm = (v: Tip | string | undefined): Tip | undefined => (typeof v === 'string' ? (v ? { text: v } : undefined) : v?.text ? v : undefined);

export const vTip: Directive<HTMLElement, Tip | string | undefined> = {
  mounted (el, binding) {
    const enter = (e: PointerEvent) => {
      const t = tips.get(el)?.tip;
      if (t && e.pointerType !== 'touch') show(el, t);
    };
    const leave = () => hideTip(el);
    // Pressing it hides it until the pointer leaves: the person knows what they chose.
    const press = () => {
      hideTip(el);
      hiddenAt = 0;
    };
    const focus = () => {
      const t = tips.get(el)?.tip;
      if (t && el.matches(':focus-visible')) show(el, t, true);
    };
    el.addEventListener('pointerenter', enter);
    el.addEventListener('pointerleave', leave);
    el.addEventListener('pointerdown', press);
    el.addEventListener('focus', focus);
    el.addEventListener('blur', leave);
    tips.set(el, {
      tip: norm(binding.value),
      off: () => {
        el.removeEventListener('pointerenter', enter);
        el.removeEventListener('pointerleave', leave);
        el.removeEventListener('pointerdown', press);
        el.removeEventListener('focus', focus);
        el.removeEventListener('blur', leave);
      },
    });
  },
  updated (el, binding) {
    const entry = tips.get(el);
    if (entry) entry.tip = norm(binding.value);
    if (owner === el && tip.open && entry?.tip) {
      tip.text = entry.tip.text;
      tip.keys = entry.tip.keys ?? '';
    }
  },
  unmounted (el) {
    tips.get(el)?.off();
    tips.delete(el);
    hideTip(el);
  },
};
