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
        'query',
        'validate',
        'sidecar',
        'repo',
        'datasource',
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
