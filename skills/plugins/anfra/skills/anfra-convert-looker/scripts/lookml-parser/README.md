# lookml-parser (copied)

A copy of [lookml-parser](https://github.com/fabio-looker/node-lookml-parser)
(MIT, see `LICENSE`). The LookML plugin uses only `parseFiles`.

The npm package needs `fs`, `glob`, `crypto` and `child_process`, so it does not
run in a browser. This copy:

- uses ES modules, not CommonJS;
- reads only in-memory files (`input: [{ path, content }]`);
- has no disk cache;
- uses small shims for `bluebird` and `path` (`shims/`);
- parses dashboard YAML with the `yaml` package, not `js-yaml`.

`upstream.json` maps each file here to its upstream file, with the version,
the commit and the kind of change. Each change that is not module syntax has a
`// vendor:` comment. The folder layout mirrors upstream `lib/`.

## Upgrade

1. Clone upstream. Find the commit of the new version.

2. Apply the upstream diff of the copied files. Run from the repository root:

   ```sh
   DIR=packages/semantic-catalog/src/plugins/lookml/lookml-parser
   SOURCES=$(node -e "
     const m = require('./$DIR/upstream.json');
     for (const f of Object.values(m.files))
       if (f.source?.startsWith('lib/') && ['esm', 'modified'].includes(f.change)) console.log(f.source)")
   git -C <upstream> diff <old-commit> <new-commit> -- $SOURCES \
     | git apply -p2 --directory=$DIR --reject
   ```

   A rejected hunk goes to a `.rej` file. Apply it by hand, then delete the
   `.rej` file. Convert a new `require()` to `import`.

3. Do a check of the upstream diff of the `local` files too (`lib/cache`,
   `lib/parse-files/file-loader.js`, `index.js`). Copy a change to the part
   that this copy keeps.

4. If `lib/parse/lookml.peg` changed, copy `lib/parse/parser.generated.js`
   from the npm tarball (`npm pack lookml-parser@<version>`). Change its last
   `module.exports = {...}` to `export {...}`.

5. If a copied file requires a new upstream file, copy it too and add it to
   `upstream.json`. A Node built-in (`fs`, `path`, `crypto`, ...) must not
   come in: add a shim or a stub.

6. Set `upstreamVersion` and `commit` in `upstream.json`.

7. Do the checks. Run from `packages/semantic-catalog`:

   ```sh
   # ES modules run in strict mode: an undeclared variable throws.
   npx eslint --no-eslintrc --no-ignore --env es2022,browser \
     --parser-options ecmaVersion:2022,sourceType:module --rule '{"no-undef":"error"}' \
     src/plugins/lookml/lookml-parser
   pnpm vitest --run src/plugins/lookml
   pnpm build
   # The bundle must not import Node modules.
   grep -nE "from ['\"](node:|fs|path|crypto|glob|child_process)" dist/plugins/lookml/index.js
   ```

   The eslint check reports `Exception` in
   `transformations/apply-extensions-refinements.js`. This is an upstream bug
   and it throws in upstream too.
