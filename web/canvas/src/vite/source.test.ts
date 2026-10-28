import { describe, expect, it } from 'vitest';
import { compileTemplate, parse } from 'vue/compiler-sfc';
import { EditError, applyEdit, describeSource, editSource, parseLoc, sourceTransform } from './source.ts';

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
    expect(describeSource(FILE, at(8, 5))).toEqual({
      tag: 'h1', opening: '<h1 class="text-title">', classes: 'text-title', text: 'Invite your team',
      attrs: [{ name: 'class', value: 'text-title' }], children: 0, first: true, last: false,
    });
    expect(describeSource(FILE, at(14, 7)).attrs).toEqual([{ name: 'key', bound: 'i' }]);
  });

  it('sets, adds and removes attributes, and leaves bound ones to the agent', () => {
    expect(applyEdit(FILE, at(9, 5), { kind: 'attr', name: 'variant', value: 'ghost' })).toContain('<Button variant="ghost">');
    expect(applyEdit(FILE, at(9, 5), { kind: 'attr', name: 'disabled', value: true })).toContain('<Button disabled variant="outline">');
    expect(applyEdit(FILE, at(9, 5), { kind: 'attr', name: 'variant', value: null })).toContain('<Button>\n      Send invites');
    expect(applyEdit(FILE, at(7, 3), { kind: 'class', to: '' })).toContain('<section>');
    expect(() => applyEdit(FILE, at(14, 7), { kind: 'attr', name: 'key', value: 'x' })).toThrow(/bound to `i`/);
  });

  it('removes, duplicates and moves an element, saying where it now is', () => {
    expect(applyEdit(FILE, at(8, 5), { kind: 'remove' })).toContain('<section class="p-4">\n    <Button');
    const dup = editSource(FILE, at(8, 5), { kind: 'duplicate' });
    expect(dup.code).toContain('Invite your team</h1>\n    <h1 class="text-title">Invite your team</h1>');
    expect(dup.at).toEqual({ file: 'src/pages/Invite.vue', line: 9, column: 5 });
    const down = editSource(FILE, at(8, 5), { kind: 'move', by: 1 });
    expect(down.code).toContain('</Button>\n    <h1 class="text-title">');
    expect(down.at).toEqual({ file: 'src/pages/Invite.vue', line: 11, column: 5 });
    expect(() => applyEdit(FILE, at(8, 5), { kind: 'move', by: -1 })).toThrow(/already the first/);
  });

  it('keeps a v-if chain whole when its first element is removed, and refuses edits that break the template', () => {
    const spaced = '<template>\n  <main>\n    <h1>A</h1>\n\n    <p>B</p>\n\n    <p>C</p>\n  </main>\n</template>\n';
    expect(applyEdit(spaced, at(5, 5), { kind: 'remove' })).toBe('<template>\n  <main>\n    <h1>A</h1>\n\n    <p>C</p>\n  </main>\n</template>\n');
    const chain = `<template>
  <main>
    <p v-if="failed">Failed</p>
    <p v-else-if="loading">Loading</p>
    <ul v-else>List</ul>
  </main>
</template>
`;
    const first = applyEdit(chain, at(3, 5), { kind: 'remove' });
    expect(first).toContain('<main>\n    <p v-if="!(failed) && (loading)">Loading</p>\n    <ul v-else>');
    const two = applyEdit(first, at(3, 5), { kind: 'remove' });
    expect(two).toContain('<main>\n    <ul v-if="!(!(failed) && (loading))">List</ul>');
    expect(applyEdit(chain, at(5, 5), { kind: 'remove' })).toContain('Loading</p>\n  </main>');
    expect(() => applyEdit(chain, at(4, 5), { kind: 'move', by: 1 })).toThrow(/would break/);
  });

  it('inserts markup beside or inside an element, with its imports', () => {
    const imports = [{ name: 'Badge', from: '@/components/ui/badge' }, { name: 'Button', from: '@/components/ui/button' }];
    const after = editSource(FILE, at(8, 5), { kind: 'insert', where: 'after', markup: '<Badge>New</Badge>', imports });
    expect(after.code).toContain("import { Button } from '@/components/ui/button';\nimport { Badge } from '@/components/ui/badge';\nconst n");
    expect(after.code).toContain('Invite your team</h1>\n    <Badge>New</Badge>');
    expect(after.at).toEqual({ file: 'src/pages/Invite.vue', line: 10, column: 5 });
    const inside = editSource(FILE, at(13, 5), { kind: 'insert', where: 'inside', markup: '<li>\n  Last\n</li>' });
    expect(inside.code).toContain('<li v-for="i in n" :key="i">Row</li>\n      <li>\n        Last\n      </li>\n    </ul>');
    expect(inside.at).toEqual({ file: 'src/pages/Invite.vue', line: 15, column: 7 });
    const bare = editSource('<template>\n  <div />\n</template>\n', at(2, 3), { kind: 'insert', where: 'inside', markup: '<Badge />', imports });
    expect(bare.code).toBe("<script setup lang=\"ts\">\nimport { Badge } from '@/components/ui/badge';\nimport { Button } from '@/components/ui/button';\n</script>\n\n<template>\n  <div>\n    <Badge />\n  </div>\n</template>\n");
    expect(bare.at).toEqual({ file: 'src/pages/Invite.vue', line: 8, column: 5 });
  });

  it('adds a name to the import of its module that is already there', () => {
    const imports = [{ name: 'Badge', from: '@/components/ui/button' }, { name: 'Card', from: '@/components/ui/card' }];
    const out = editSource(FILE, at(8, 5), { kind: 'insert', where: 'after', markup: '<Badge /><Card />', imports });
    expect(out.code).toContain("import { Button, Badge } from '@/components/ui/button';\nimport { Card } from '@/components/ui/card';\nconst n");
    expect(out.at).toEqual({ file: 'src/pages/Invite.vue', line: 10, column: 5 });
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
