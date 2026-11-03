import { describe, expect, it } from 'vitest';
import { PROVISION_ELEMENT_ID, type Provision } from '../common/bridge';
import { salesDataset, testUser } from '../app/testSupport';
import {
  definitionPosition, definitionStack, provisionDocument, provisionFrame,
} from './provision';

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

  it('names the frame script, so devtools tell the SDK\'s code from the app\'s', () => {
    const doc = provisionDocument('<head></head><script>author()</script>', options);
    expect(doc).toMatch(/window\.FRAME = 1;[\s\S]*\n\/\/# sourceURL=anfra-sdk\.js<\/script>/);
    expect(doc.indexOf('sourceURL')).toBeLessThan(doc.indexOf('author()'));
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

  it('puts the policy ahead of every script however the definition starts', () => {
    const parse = (definition: string) => new DOMParser().parseFromString(provisionDocument(definition, options), 'text/html');
    for (const definition of [
      // A '>' quoted in an attribute: the policy must not land inside its value.
      '<!doctype html><html data-x=">"><head><meta charset="utf-8"><script>early()</script></head></html>',
      `<!doctype html><html><head data-x='>'><script>early()</script></head></html>`,
      // A quote in a name opens no value: the tag ends at the first '>'.
      '<html a"><script>early()</script><head x=">"></head></html>',
      '<html a="1"b\'><script>early()</script><head x=\'>\'></head></html>',
      // Comments the parser closes sooner than the first '-->'.
      '<!--><script>early()</script><!-- -->\n<!doctype html><html><head></head></html>',
      '<!---><script>early()</script><!-- -->\n<html><head></head></html>',
      '<!-- a --!><script>early()</script><!-- -->\n<html><head></head></html>',
    ]) {
      const doc = parse(definition);
      const policy = doc.querySelector('meta[http-equiv="Content-Security-Policy"]');
      expect(policy, definition).not.toBeNull();
      for (const script of doc.querySelectorAll('script')) {
        expect(policy!.compareDocumentPosition(script) & Node.DOCUMENT_POSITION_FOLLOWING, definition).toBeTruthy();
      }
    }
  });

  it('reads a long unclosed tag without backtracking through it', () => {
    // Each would take minutes if a name or a value could be split every way.
    for (const definition of [`<html ${'a'.repeat(5000)}`, `<html x=${'a'.repeat(5000)}`, `<head ${'a=b '.repeat(2000)}`]) {
      expect(provisionDocument(definition, options).startsWith(POLICY)).toBe(true);
    }
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

describe('provisionFrame', () => {
  /** Where `text` is, as a stack names it: its 1-based line and column. */
  const where = (doc: string, text: string) => {
    const at = doc.indexOf(text);
    const before = doc.slice(0, at);
    return { line: before.split('\n').length, column: at - before.lastIndexOf('\n') };
  };
  const definitions = {
    'a charset': '<!doctype html>\n<html>\n<head>\n  <meta charset="utf-8"> <script>m1()</script>\n  <title>T</title>\n</head>\n<body>\n<script>\n  m2();\n</script>\n</body>\n</html>',
    '<head> only': '<!-- c -->\n<html lang="en"><head><script>m1()</script>\n<script>m2()</script></head></html>',
    'no <head>': '<p>x</p>\n<script>m1()</script>\n\n<script>m2()</script>',
    'a preamble with code after it': '<!doctype html><html><head><script>m1()</script><meta charset="utf-8">\n<script>m2()</script>',
  };
  for (const [name, definition] of Object.entries(definitions)) {
    it(`names each line and column of the definition's, with ${name}`, () => {
      const { html, lines } = provisionFrame(definition, options);
      expect(html).toBe(provisionDocument(definition, options));
      for (const marker of ['m1()', 'm2()']) {
        const { line, column } = where(html, marker);
        expect(definitionPosition(lines, line, column)).toEqual(where(definition, marker));
      }
      expect(definitionPosition(lines, where(html, 'window.FRAME').line, 1)).toBeUndefined();
    });
  }

  it('rewrites a stack to the definition\'s lines, and the SDK\'s to its name', () => {
    const definition = '<!doctype html>\n<head>\n<meta charset="utf-8">\n</head>\n<script>\n  boom();\n</script>';
    const { html, lines } = provisionFrame(definition, options);
    const boom = where(html, 'boom()');
    const sdk = where(html, 'window.FRAME');
    const stack = `Error: x\n    at about:srcdoc:${boom.line}:${boom.column}\n    at Le.notify (about:srcdoc:${sdk.line}:9)\n    at https://cdn.jsdelivr.net/npm/echarts@5.5.1/dist/echarts.min.js:1:2`;
    expect(definitionStack(stack, lines)).toBe('Error: x\n    at definition:6:3\n    at Le.notify (anfra-sdk.js)\n    at https://cdn.jsdelivr.net/npm/echarts@5.5.1/dist/echarts.min.js:1:2');
  });
});
