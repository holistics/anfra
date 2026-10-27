// @vitest-environment jsdom
import { describe, expect, it } from 'vitest';
import { snapshot } from './snapshot.ts';

describe('snapshot', () => {
  it('is the page with its styles and what is typed in it, as an SVG image', () => {
    document.head.innerHTML = '<style>.a > b { color: red }</style>';
    document.documentElement.className = 'dark';
    document.body.innerHTML = '<p class="a"><b>Hi & bye</b></p><input value=""><script>alert(1)</script>';
    document.querySelector('input')!.value = 'typed';
    const svg = snapshot(document);
    expect(svg).toMatch(/^<svg xmlns="http:\/\/www.w3.org\/2000\/svg"/);
    expect(svg).toContain('.a &gt; b');
    expect(svg).toContain('class="dark"');
    expect(svg).toContain('Hi &amp; bye');
    expect(svg).toContain('value="typed"');
    expect(svg).not.toContain('alert');
    expect(new DOMParser().parseFromString(svg, 'image/svg+xml').querySelector('parsererror')).toBeNull();
  });
});
