import { describe, expect, it } from 'vitest';
import { PROVISION_ELEMENT_ID, type Provision } from '../common/bridge';
import { salesDataset, testUser } from '../app/testSupport';
import { provisionDocument } from './provision';

const provision: Provision = { datasets: { sales: salesDataset }, user: testUser, hostOrigin: 'http://127.0.0.1:7878' };
const options = { baseHref: '/appserve/files/sales/', provision, frameScript: 'window.FRAME = 1;' };

describe('provisionDocument', () => {
  it('puts the base, the provision and the frame script ahead of the definition, after its charset', () => {
    const doc = provisionDocument('<html><head><meta charset="utf-8"><title>T</title><script>author()</script></head></html>', options);
    const at = (s: string) => doc.indexOf(s);
    expect(at('<meta charset="utf-8">')).toBeLessThan(at('<base href="/appserve/files/sales/">'));
    expect(at('<base')).toBeLessThan(at(`id="${PROVISION_ELEMENT_ID}"`));
    expect(at(`id="${PROVISION_ELEMENT_ID}"`)).toBeLessThan(at('window.FRAME = 1;'));
    expect(at('window.FRAME = 1;')).toBeLessThan(at('author()'));
  });

  it('goes after <head> without a charset, and first without a <head>', () => {
    expect(provisionDocument('<head lang="en"><script>a()</script></head>', options).startsWith('<head lang="en">\n<base')).toBe(true);
    expect(provisionDocument('<script>a()</script>', options).startsWith('<base')).toBe(true);
  });

  it('keeps what it injects from closing its own element', () => {
    const doc = provisionDocument('<head></head>', {
      ...options,
      baseHref: '/a"b/',
      provision: { ...provision, datasets: { s: { ...salesDataset, label: '</script><script>alert(1)</script>' } } },
      frameScript: 'const s = "</script>";',
    });
    expect(doc).toContain('<base href="/a&quot;b/">');
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
