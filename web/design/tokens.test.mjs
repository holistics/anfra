// What keeps the token files whole: every reference resolves, dark redefines only light's Tier 2
// tokens, and nothing outside Tier 1 holds a raw colour.
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import assert from 'node:assert/strict';

const read = (file) => readFileSync(new URL(file, import.meta.url), 'utf8').replace(/\/\*[\s\S]*?\*\//g, '').replace(/@import[^;]+;/g, '');
const css = ['tokens.css', 'fonts.css', 'type.css'].map(read).join('\n');

/** The custom properties each rule sets, by selector. */
function blocks (text) {
  const out = [];
  for (const [, selector, body] of text.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    const props = new Map();
    for (const [, name, value] of body.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)) props.set(name, value.trim());
    out.push({ selector: selector.trim(), props });
  }
  return out;
}

const TIER_1 = /^--anfra-(ink|paper|violet|red|amber|green|white|black)-/;
const all = blocks(css);
const light = new Map(all.filter((b) => b.selector === ':root').flatMap((b) => [...b.props]));
const dark = all.find((b) => b.selector.includes('.dark'))?.props ?? new Map();

test('every var() names a token defined here', () => {
  for (const { props } of all) {
    for (const value of props.values()) {
      for (const [, name] of value.matchAll(/var\(\s*(--[\w-]+)/g)) assert.ok(light.has(name), `${name} is not defined`);
    }
  }
});

test('dark redefines only Tier 2 tokens light has', () => {
  assert.ok(dark.size > 0);
  for (const name of dark.keys()) {
    assert.ok(light.has(name), `${name} is set for dark only`);
    assert.doesNotMatch(name, TIER_1, `${name}: dark redefines Tier 2 only`);
  }
});

test('only Tier 1 holds raw colours', () => {
  for (const [name, value] of [...light, ...dark]) {
    if (TIER_1.test(name) || /^--anfra-e\d$/.test(name)) continue; // shadows: ink at a low alpha
    assert.doesNotMatch(value, /oklch\(|#[0-9a-f]{3,8}\b|rgb\(/i, `${name} holds a raw colour: point it at a Tier 1 token`);
  }
});
