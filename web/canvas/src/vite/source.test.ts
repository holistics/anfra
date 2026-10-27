import { describe, expect, it } from 'vitest';
import { compileTemplate, parse } from 'vue/compiler-sfc';
import { EditError, applyEdit, describeSource, parseLoc, sourceTransform } from './source.ts';

const FILE = `<script setup lang="ts">
import { Button } from '@/components/ui/button';
const n = 2;
</script>

<template>
  <section class="p-4">
    <h1 class="text-title">Invite your team</h1>
    <Button variant="outline">
      Send invites
    </Button>
    <p>{{ n }} seats left</p>
    <ul>
      <li v-for="i in n" :key="i">Row</li>
    </ul>
  </section>
</template>
`;
const at = (line: number, column: number) => ({ file: 'src/pages/Invite.vue', line, column });

describe('applyEdit', () => {
  it('changes an element\'s plain text, keeping the whitespace around it', () => {
    const out = applyEdit(FILE, at(8, 5), { kind: 'text', from: 'Invite your team', to: 'Invite <teammates>' });
    expect(out).toContain('<h1 class="text-title">Invite &lt;teammates></h1>');
    const slot = applyEdit(FILE, at(9, 5), { kind: 'text', from: 'Send invites', to: 'Send' });
    expect(slot).toContain('<Button variant="outline">\n      Send\n    </Button>');
  });

  it('finds elements inside v-for', () => {
    expect(applyEdit(FILE, at(14, 7), { kind: 'text', from: 'Row', to: 'Seat' })).toContain('<li v-for="i in n" :key="i">Seat</li>');
  });

  it('refuses text with bindings in it, and text that changed', () => {
    expect(() => applyEdit(FILE, at(12, 5), { kind: 'text', from: '2 seats left', to: 'x' })).toThrow(EditError);
    expect(() => applyEdit(FILE, at(8, 5), { kind: 'text', from: 'Something else', to: 'x' })).toThrow(/not what the screen showed/);
    expect(() => applyEdit(FILE, at(99, 1), { kind: 'text', from: 'a', to: 'b' })).toThrow(/no longer where it was/);
  });

  it('replaces or adds static classes', () => {
    expect(applyEdit(FILE, at(7, 3), { kind: 'class', to: 'p-6  gap-2' })).toContain('<section class="p-6 gap-2">');
    expect(applyEdit(FILE, at(13, 5), { kind: 'class', to: 'grid' })).toContain('<ul class="grid">');
  });

  it('describes an element for the inspector', () => {
    expect(describeSource(FILE, at(8, 5))).toEqual({ tag: 'h1', opening: '<h1 class="text-title">', classes: 'text-title', text: 'Invite your team' });
  });

  it('reads a place in a file', () => {
    expect(parseLoc('src/a b.vue:12:5')).toEqual({ file: 'src/a b.vue', line: 12, column: 5 });
    expect(parseLoc('nowhere')).toBeUndefined();
  });
});

describe('sourceTransform', () => {
  it('tags elements and components with where they are written', () => {
    const { descriptor } = parse(FILE, { filename: '/repo/web/src/pages/Invite.vue' });
    const { code } = compileTemplate({
      id: 'x',
      filename: '/repo/web/src/pages/Invite.vue',
      source: descriptor.template!.content,
      ast: descriptor.template!.ast,
      compilerOptions: { nodeTransforms: [sourceTransform('/repo/web')] },
    });
    expect(code).toContain('"data-canvas-src": "src/pages/Invite.vue:8:5"');
    expect(code).toContain('"data-canvas-src": "src/pages/Invite.vue:9:5"');
    expect(code).toContain('"data-canvas-cmp": "Button"');
  });
});
