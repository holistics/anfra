// The renderer's script, built as an IIFE into `host` (`anfra-sdk:renderer`, tsup.config.ts). A frame
// injects it on its first rendering; it installs itself where the frame looks for it.
import { renderer } from './renderer';
import { RENDERER_GLOBAL } from './types';

(globalThis as unknown as Record<string, unknown>)[RENDERER_GLOBAL] = renderer;
