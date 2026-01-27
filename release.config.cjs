const config = {
  branches: ['master'],
  
  plugins: [
    [
      '@semantic-release/commit-analyzer',
      {
        preset: 'conventionalcommits',
        releaseRules: [
          { release: 'minor', type: 'feat' },
          { release: 'patch', type: 'fix' },
          { release: 'patch', type: 'perf' },
          { release: 'patch', type: 'style' },
          { release: 'patch', type: 'refactor' },
          { release: 'patch', type: 'build' },
          { release: 'patch', scope: 'README', type: 'docs' },
          { release: 'patch', scope: 'README.md', type: 'docs' },
          { release: false, type: 'docs' },
          { release: false, type: 'test' },
          { release: false, type: 'ci' },
          { release: false, type: 'chore' },
          { release: false, type: 'wip' },
          { release: 'major', type: 'BREAKING CHANGE' },
          { release: 'major', scope: 'BREAKING CHANGE' },
          { release: 'major', subject: '*BREAKING CHANGE*' },
          { release: 'patch', subject: '*force release*' },
          { release: 'patch', subject: '*force patch*' },
          { release: 'minor', subject: '*force minor*' },
          { release: 'major', subject: '*force major*' },
          { release: false, subject: '*skip release*' },
        ],
      },
    ],
    '@semantic-release/release-notes-generator',
    [
      '@semantic-release/changelog',
      {
        changelogFile: 'CHANGELOG.md',
      },
    ],
    '@semantic-release/npm', // Updates root package.json
    [
      '@semantic-release/exec',
      {
        prepareCmd:
          'npm version ${nextRelease.version} --workspaces --no-git-tag-version --allow-same-version || (npm version ${nextRelease.version} --no-git-tag-version --prefix frontend && npm version ${nextRelease.version} --no-git-tag-version --prefix backend)',
      },
    ],
    [
      '@semantic-release/git',
      {
        assets: [
          'CHANGELOG.md',
          'package.json',
          'package-lock.json',
          'npm-shrinkwrap.json',
          'frontend/package.json',
          'frontend/package-lock.json',
          'backend/package.json',
          'backend/package-lock.json',
        ],
        message: 'chore(release): ${nextRelease.version} [skip ci]\n\n${nextRelease.notes}',
      },
    ],
    '@semantic-release/github',
  ],
};

module.exports = config;
