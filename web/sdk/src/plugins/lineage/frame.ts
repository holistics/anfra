/** `lineage`'s frame half: `Anfra.lineage()`. */
import type { FrameKernel } from '../../app/kernel';
import type { LineageOptions, LineageResult, LineageTarget } from '../../common/types';

export default function setup (kernel: FrameKernel): void {
  /**
   * Trace fields through the repo's catalog: what each one is built from (the fields, models and
   * sources upstream of it), as a diagram to draw. Name each field by its parent and its name in
   * it: `{ parent: 'orders', child: 'amount' }` for a model's field, `{ parent: 'ecommerce', child:
   * 'revenue' }` for a dataset's metric. Nothing needs ingesting first: the host builds the repo's
   * catalog when it has to. A field that cannot be traced answers its `error`, and the others still
   * answer.
   */
  function lineage (targets: LineageTarget | readonly LineageTarget[], options: LineageOptions = {}): Promise<LineageResult> {
    const list: LineageTarget[] = Array.isArray(targets) ? [...targets] : [targets as LineageTarget];
    const named = (s: unknown) => typeof s === 'string' && s.trim() !== '';
    if (list.length === 0 || list.some((t) => typeof t !== 'object' || t === null || !named(t.parent) || !named(t.child))) {
      return Promise.reject(new kernel.errors.ValidationError(
        "Anfra.lineage() takes a target or an array of targets, each by its parent and its name in it, such as { parent: 'ecommerce', child: 'revenue' }.",
      ));
    }
    const { limit, offset, signal } = options;
    return kernel.call<LineageResult>('lineage.trace', {
      targets: list.map(({ parent, child }) => ({ parent, child })),
      ...(limit !== undefined ? { limit } : {}),
      ...(offset !== undefined ? { offset } : {}),
    }, signal);
  }
  kernel.provide(lineage);
}
