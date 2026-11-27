const config = {
  branches: ['master'],
  
  plugins: [
    [
      '@semantic-release/commit-analyzer',
      {
        preset: 'conventionalcommits',
        releaseRules: [
          { release: 'patch', type: 'feat' },
          { release: 'patch', type: 'fix' },
          { release: 'patch', type: 'perf' },
          { release: 'patch', type: 'style' },
          { release: 'patch', type: 'refactor' },
          { release: 'patch', type: 'build' },
          { release: 'patch', scope: 'README', type: 'docs' },
          { release: 'patch', scope: 'README.md', type: 'docs' },
          { release: 'patch', type: 'docs' },
          { release: 'patch', type: 'test' },
          { release: 'patch', type: 'ci' },
          { release: 'patch', type: 'chore' },
          { release: 'patch', type: 'wip' },
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
    // Root package.json is the single source of the release version: the
    // Docker build (Dockerfile, VERSION build arg default) and `make build`
    // read it and stamp it into the Go binary via -ldflags.
    '@semantic-release/npm',
    [
      '@semantic-release/exec',
      {
        // The dashboard shows its own package version (__APP_VERSION__); keep it in step.
        prepareCmd:
          'npm version ${nextRelease.version} --no-git-tag-version --allow-same-version --prefix frontend',
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
        ],
        message: 'chore(release): ${nextRelease.version} [skip ci]\n\n${nextRelease.notes}',
      },
    ],
    '@semantic-release/github',
  ],
};

module.exports = config;
