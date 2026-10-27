// What an agent reads: comments and picked elements as Markdown, each saying what the person
// pointed at (the frame, its route and scenario, and where the element is written), so the agent
// can find the code without seeing the screen.
import type { Board, Comment, ElementRef, Frame } from './board.ts';
import { find } from './ops.ts';

const quote = (text: string) => text.split('\n').map((l) => `> ${l}`).join('\n');
const short = (text: string, max = 80) => (text.length > max ? `${text.slice(0, max - 1)}…` : text);

function frameLine (frame: Frame | undefined): string {
  if (!frame) return '- Frame: (removed)';
  const scenario = frame.scenario ? `, scenario \`${frame.scenario}\`` : ', live backend';
  return `- Frame: ${frame.name} (\`${frame.route}\`${scenario})`;
}

/** One element, as a handle and the lines that locate it. */
export function elementMarkdown (board: Board, el: ElementRef): string {
  const frame = find(board, el.frame) as Frame | undefined;
  const what = el.component ? `<${el.component}>` : `<${el.tag}>`;
  const lines = [`- Element: \`${what}\`${el.text ? ` "${short(el.text)}"` : ''}`];
  if (el.source) lines.push(`- Source: \`${el.source}\`${el.index ? ` (occurrence ${el.index + 1} on screen)` : ''}`);
  else if (el.selector) lines.push(`- Selector: \`${el.selector}\``);
  lines.push(frameLine(frame));
  return lines.join('\n');
}

function where (board: Board, c: Comment): string {
  if (c.anchor.type === 'element') return elementMarkdown(board, c.anchor);
  const { x, y } = c.anchor;
  const near = board.frames.find((f) => x >= f.x && x <= f.x + f.width && y >= f.y - 60 && y <= f.y + f.height);
  return near ? frameLine(near) : `- On the canvas at (${Math.round(x)}, ${Math.round(y)})`;
}

export function commentMarkdown (board: Board, c: Comment, n?: number): string {
  const title = `### ${n ? `${n}. ` : ''}${short(c.body.split('\n')[0], 60)}`;
  const parts = [title, `- Id: \`${c.id}\` (${c.status})`, where(board, c), '', quote(c.body)];
  for (const r of c.replies) parts.push('', `**${r.author}:**`, quote(r.body));
  return parts.join('\n');
}

/** The comments to act on, as one task for an agent. */
export function feedbackMarkdown (board: Board, comments: Comment[] = board.comments.filter((c) => c.status === 'open')): string {
  if (!comments.length) return `No open feedback on ${board.name}.`;
  const head = [
    `## Feedback on ${board.name}`,
    '',
    `${comments.length} comment${comments.length === 1 ? '' : 's'}. Each says what was pointed at and where it is written. ` +
    'Change the source, then resolve each comment with a short note of what you did.',
  ];
  return [...head, ...comments.map((c, i) => `\n${commentMarkdown(board, c, i + 1)}`)].join('\n');
}
