const config = {
  branches: ['master'],
  plugins: [
    '@semantic-release/commit-analyzer',
    '@semantic-release/release-notes-generator',
    '@semantic-release/git', {
        "assets": [
            "CHANGELOG.md",
            "package.json",
            "package-lock.json",
            "npm-shrinkwrap.json",
            "frontend/package.json",
            "frontend/package-lock.json",
            "backend/package.json",
            "backend/package-lock.json",
        ],
        "message": "chore(release): ${nextRelease.version} [skip ci]\n\n${nextRelease.notes}",
    },
    '@semantic-release/github'
  ],
};

module.exports = config;
