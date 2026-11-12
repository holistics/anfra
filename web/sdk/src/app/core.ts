/**
 * `core`'s frame side, which the kernel is: the host's refresh, `core.execute`, run on every app in
 * the frame. Nothing here is the definition's: it refreshes its own app with `app.refresh()`.
 */
import type { ExecuteSummary } from '../common/types';
import type { FrameKernel } from './kernel';

/** Start answering the host's `core.*` calls in this frame; `kernel` is the kernel's, named `core`. */
export function installCore (kernel: FrameKernel): void {
  kernel.handle('execute', async (): Promise<ExecuteSummary> => {
    const summaries = await Promise.all(kernel.sdk.apps.map((app) => app.refresh()));
    return {
      succeeded: summaries.flatMap((summary) => summary.succeeded),
      failed: summaries.flatMap((summary) => summary.failed),
    };
  });
}
