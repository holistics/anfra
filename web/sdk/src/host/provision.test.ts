import { describe, expect, it } from 'vitest';
import { PROVISION_ELEMENT_ID, type Provision } from '../common/bridge';
import { salesDataset, testUser } from '../app/testSupport';
import { provisionDocument } from './provision';

const provision: Provision = { datasets: { sales: salesDataset }, user: testUser, plugins: [], hostOrigin: 'http://127.0.0.1:7878' };
const options = { contentPolicy: "default-src 'none'", provision, frameScript: 'window.FRAME = 1;' };
const POLICY = `<meta http-equiv="Content-Security-Policy" content="default-src 'none'">`;

describe('provisionDocument', () => {
  it('puts the provision and the frame script ahead of the definition, after its charset', () => {
    const doc = provisionDocument('<html><head><meta charset="utf-8"><title>T</title><script>author()</script></head></html>', options);
    const at = (s: string) => doc.indexOf(s);
    expect(at('<meta charset="utf-8">')).toBeLessThan(at(`id="${PROVISION_ELEMENT_ID}"`));
    expect(at(`id="${PROVISION_ELEMENT_ID}"`)).toBeLessThan(at('window.FRAME = 1;'));
    expect(at('window.FRAME = 1;')).toBeLessThan(at('author()'));
  });

  it('goes after <head> without a charset, and first without a <head>', () => {
    expect(provisionDocument('<head lang="en"><script>a()</script></head>', options)).toMatch(/^<head lang="en"><meta [^>]*>\n<script type="application\/json"/);
    expect(provisionDocument('<header>h</header><script>a()</script>', options)).toMatch(/^<meta [^>]*>\n<script type="application\/json"/);
  });

  it('puts the policy first, before anything of the definition but its doctype, <html> and <head>', () => {
    const doc = provisionDocument('<!-- c -->\n<!DOCTYPE html>\n<html lang="en"><head><script>early()</script><meta charset="utf-8"></head></html>', options);
    expect(doc.startsWith(`<!-- c -->\n<!DOCTYPE html>\n<html lang="en"><head>${POLICY}\n<script>early()`)).toBe(true);
    expect(provisionDocument('<!doctype html><p>x</p>', options).startsWith(`<!doctype html>${POLICY}\n<script`)).toBe(true);
    // Something ahead of <head>: the policy goes before it, so nothing runs ahead of the policy.
    expect(provisionDocument('<html><script>early()</script><head></head></html>', options).startsWith(`<html>${POLICY}\n<script>early()`)).toBe(true);
  });

  it('keeps the definition\'s <html> and <head> attributes, the policy the first element parsed', () => {
    const parse = (definition: string) => new DOMParser().parseFromString(provisionDocument(definition, options), 'text/html');
    for (const definition of [
      '<!doctype html><html lang="en" class="dark"><head data-x="1"><script>early()</script><meta charset="utf-8"></head><body></body></html>',
      '<html lang="en" class="dark">\n  <!-- the app -->\n  <head data-x="1">\n<title>T</title></head></html>',
    ]) {
      const doc = parse(definition);
      expect(doc.documentElement.getAttribute('lang')).toBe('en');
      expect(doc.documentElement.className).toBe('dark');
      expect(doc.head.getAttribute('data-x')).toBe('1');
      expect(doc.head.firstElementChild?.getAttribute('http-equiv')).toBe('Content-Security-Policy');
    }
    // An <html> after the policy still lends its attributes to the root.
    expect(parse('<script>early()</script><html lang="en"><body></body></html>').documentElement.getAttribute('lang')).toBe('en');
  });

  it('puts no base in the document', () => {
    expect(provisionDocument('<head></head>', options)).not.toContain('<base');
  });

  it('keeps what it injects from closing its own element', () => {
    const doc = provisionDocument('<head></head>', {
      ...options,
      contentPolicy: `a"b<c`,
      provision: { ...provision, datasets: { s: { ...salesDataset, label: '</script><script>alert(1)</script>' } } },
      frameScript: 'const s = "</script>";',
    });
    expect(doc).toContain('content="a&quot;b&lt;c"');
    expect(doc).toContain('\\u003c/script\\u003e\\u003cscript\\u003ealert(1)');
    expect(doc).toContain('const s = "<\\/script>";');
    expect(doc.match(/<\/script>/g)).toHaveLength(2); // the two it opened, closed once each
  });

  it('carries the provision as JSON a frame can read back', () => {
    const doc = provisionDocument('<head></head>', options);
    const json = doc.slice(doc.indexOf('>', doc.indexOf(PROVISION_ELEMENT_ID)) + 1, doc.indexOf('</script>'));
    expect(JSON.parse(json)).toEqual(provision);
  });
});
