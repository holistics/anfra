// @vitest-environment node
// Node's Blob reads back; jsdom's does not.
import { describe, expect, it } from 'vitest';
import { pdfOfJpeg } from './pdf';

async function text (blob: Blob): Promise<string> {
  return new TextDecoder('latin1').decode(new Uint8Array(await blob.arrayBuffer()));
}

describe('pdfOfJpeg', () => {
  const jpeg = new Uint8Array([0xff, 0xd8, 0xff, 0xe0, 1, 2, 3, 0xff, 0xd9]);

  it('is one page the picture’s size in points, holding the JPEG as it is', async () => {
    const blob = pdfOfJpeg(jpeg, { width: 1600, height: 800 }, { width: 800, height: 400 });
    const pdf = await text(blob);

    expect(blob.type).toBe('application/pdf');
    expect(pdf.startsWith('%PDF-1.4\n')).toBe(true);
    expect(pdf).toContain('/MediaBox [0 0 600.00 300.00]');
    expect(pdf).toContain('/Width 1600 /Height 800');
    expect(pdf).toContain('/Filter /DCTDecode /Length 9');
    expect(pdf).toContain('q 600.00 0 0 300.00 0 0 cm /Im0 Do Q');
    expect(pdf.trimEnd().endsWith('%%EOF')).toBe(true);
  });

  it('points its cross-reference table at each object', async () => {
    const pdf = await text(pdfOfJpeg(jpeg, { width: 2, height: 2 }, { width: 1, height: 1 }));
    const offsets = [...pdf.matchAll(/^(\d{10}) 00000 n $/gm)].map((m) => Number(m[1]));
    expect(offsets).toHaveLength(5);
    offsets.forEach((offset, i) => expect(pdf.slice(offset).startsWith(`${i + 1} 0 obj`)).toBe(true));
    const startxref = Number(/startxref\n(\d+)/.exec(pdf)?.[1]);
    expect(pdf.slice(startxref).startsWith('xref')).toBe(true);
  });
});
