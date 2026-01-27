const config = {
  branches: ['master'],
  
  plugins: [
    '@semantic-release/commit-analyzer',
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
