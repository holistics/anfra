// What keeps the class layer and the icons in step with the tokens: components.css reads only
// Tier 2 tokens and type roles that tokens.css and type.css define, and holds no colour of its own;
// icons.js is what scripts/icons.mjs writes from lucide.
import { readFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { ICONS } from './icons.js';

const read = (file) => readFileSync(new URL(file, import.meta.url), 'utf8').replace(/\/\*[\s\S]*?\*\//g, '');
const defined = new Set([...['tokens.css', 'type.css'].map(read).join('\n').matchAll(/(--anfra-[\w-]+)\s*:/g)].map((m) => m[1]));
const components = read('components.css');
const TIER_1 = /^--anfra-(ink|paper|violet|red|amber|green|white|black)-/;

test('components.css reads only defined Tier 2 tokens', () => {
  const used = new Set([...components.matchAll(/var\(\s*(--[\w-]+)/g)].map((m) => m[1]));
  assert.ok(used.size > 0);
  for (const name of used) {
    assert.ok(defined.has(name), `${name} is not defined`);
    assert.doesNotMatch(name, TIER_1, `${name}: the class layer reads Tier 2 tokens, not Tier 1`);
  }
});

test('components.css holds no colour of its own', () => {
  assert.doesNotMatch(components, /oklch\(|#[0-9a-f]{3,8}\b|rgba?\(|hsla?\(/i);
});

test('icons.js is what scripts/icons.mjs writes', () => {
  execFileSync(process.execPath, [new URL('scripts/icons.mjs', import.meta.url).pathname, '--check']);
  for (const [name, d] of Object.entries(ICONS)) assert.match(d, /^[Mm]/, `${name} has no path`);
});

// Nested radii: a part inside a padded container takes the container's radius less the padding, so
// the corners stay concentric. Each pair is [container, part]; the container's padding is even.
const NESTED = [['.a-tabs', '.a-tab'], ['.a-menu', '.a-menu-item']];
const radii = Object.fromEntries([...read('type.css').matchAll(/(--anfra-radius-[\w-]+)\s*:\s*(\d+)px/g)].map((m) => [m[1], Number(m[2])]));
const rule = (selector) => {
  const body = components.match(new RegExp(`(?:^|\\n)${selector.replace('.', '\\.')} \\{([^}]*)\\}`))?.[1];
  assert.ok(body, `no rule for ${selector}`);
  const value = (property) => body.match(new RegExp(`(?:^|\\s)${property}:\\s*([^;]+);`))?.[1].trim();
  return { radius: value('border-radius'), padding: value('padding') };
};
const px = (value) => Number(value.replace(/var\((--[\w-]+)\)/g, (_, name) => radii[name])
  .replace(/^calc\((.*)\)$/, '$1').replace(/px/g, '').split(/\s+-\s+/).reduce((a, b) => a - b));

test('a part nested in a padded container keeps its corner concentric', () => {
  for (const [outer, inner] of NESTED) {
    const container = rule(outer);
    const part = rule(inner);
    assert.match(container.padding, /^\d+px$/, `${outer}'s padding is one length`);
    assert.equal(px(part.radius), px(container.radius) - px(container.padding), `${inner}'s radius is ${outer}'s less its padding`);
  }
});
