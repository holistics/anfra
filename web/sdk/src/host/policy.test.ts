import { describe, expect, it } from 'vitest';
import { checkOrigin, contentPolicy } from './policy';

const directive = (policy: string, name: string) => policy.split('; ').find((d) => d.startsWith(`${name} `));

describe('contentPolicy', () => {
  it('keeps the frame off the network but for the CDNs it loads from', () => {
    const policy = contentPolicy();
    expect(directive(policy, 'default-src')).toBe("default-src 'none'");
    expect(directive(policy, 'script-src')).toContain('https://cdn.jsdelivr.net');
    expect(directive(policy, 'connect-src')).toContain('https://fonts.gstatic.com');
    expect(directive(policy, 'connect-src')).not.toMatch(/https:(\s|$)|\*/);
    expect(directive(policy, 'base-uri')).toBe("base-uri 'none'");
    expect(directive(policy, 'form-action')).toBe("form-action 'none'");
    expect(policy).not.toContain("'self'");
  });

  it('adds a host\'s origins to what an app loads and fetches, once each', () => {
    const policy = contentPolicy(['https://cdn.acme.com', 'https://cdn.jsdelivr.net']);
    for (const name of ['script-src', 'style-src', 'font-src', 'connect-src']) {
      expect(directive(policy, name)).toContain('https://cdn.acme.com');
      expect(directive(policy, name)!.split(' ').filter((s) => s === 'https://cdn.jsdelivr.net')).toHaveLength(1);
    }
    expect(directive(policy, 'frame-src')).toBe("frame-src 'none'");
  });
});

describe('checkOrigin', () => {
  it('takes an https origin', () => {
    expect(checkOrigin('https://cdn.acme.com')).toBe('https://cdn.acme.com');
    expect(checkOrigin('https://cdn.acme.com:8443')).toBe('https://cdn.acme.com:8443');
  });

  it.each([
    'http://cdn.acme.com', 'https://cdn.acme.com/', 'https://cdn.acme.com/lib', 'https://*.acme.com', '*',
    'https:', 'https://CDN.acme.com', "https://a.com; connect-src *", 'https://a.com https://b.com', 'data:', 42,
  ])('refuses %j', (origin) => {
    expect(() => checkOrigin(origin)).toThrow(/https:\/\/<host>/);
  });
});
