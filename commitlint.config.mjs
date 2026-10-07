export default {
  extends: [
    '@commitlint/config-conventional',
  ],
  // https://commitlint.js.org/#/reference-rules
  // E.g. https://github.com/conventional-changelog/commitlint/blob/v17.4.4/@commitlint/config-conventional/index.js
  rules: {
    'scope-enum': [
      2,
      'always',
      [
        'cli',
        'serve',

        'engine',
        'engine.query',
        'engine.validate',
        'engine.sidecar',
        'engine.repo',
        'engine.datasource',

        'shared',
        'shared.appstep',
        'shared.apperr',
        'shared.apptracing',

        'sdk',
        'sdk.common',
        'sdk.api',
        'sdk.host',
        'sdk.app',

        'meta',
        'deps',
      ],
    ],
    'type-enum': [
      2,
      'always',
      [
        'feat',
        'fix',
        'perf',
        'docs',
        'style',
        'chore',
        'refactor',
        'test',
        'build',
        'ci',
        'security',
        'release',
      ],
    ],
  },
  defaultIgnores: false,
  ignores: [(commit) => commit.startsWith('Merge pull request')],
  prompt: {
    settings: {
      enableMultipleScopes: true,
    },
  },
};
