// `./ambient`, as the package ships it: ambient.d.ts reads `Sdk` from the sources, which the
// package leaves out, so its copy in dist reads it from the built declarations. Run after tsup:
// its declaration build clears dist's .d.ts files.
import { readFile, writeFile } from 'node:fs/promises';

const ambient = await readFile('ambient.d.ts', 'utf8');
const shipped = ambient.replace("from './src/app/sdk'", "from './app.js'");
if (shipped === ambient) throw new Error("ambient.d.ts no longer imports './src/app/sdk': update scripts/ambient.mjs");
await writeFile('dist/ambient.d.ts', shipped);
