// The annotations example's plugin: a frame half only, which sets annotations from trivial rules on
// every query's result. A host installs it beside `core` to try a definition's drawing of them.
import { definePlugin, type FrameHalf, type Plugin } from '@holistics/anfra-sdk/host';

/** `frame` is the frame half as built (`buildFrameHalf`); the package's entry passes it in. */
export function createAnnotationsExamplePlugin (frame: FrameHalf): Plugin {
  return definePlugin({
    name: 'annotations-example',
    version: '1.0.0',
    requires: ['core'],
    frame,
  });
}
