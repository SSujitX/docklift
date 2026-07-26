# Auto-Deploy

Enable automated deployments so your application rebuilds when you push to the selected GitHub branch.

## How it works

Docklift registers for GitHub `push` webhooks. When you push to the branch configured on the project, GitHub notifies your Docklift instance. Docklift verifies the webhook secret, applies a short **10 second debounce** (so burst pushes collapse into one deploy), then triggers a fresh deployment.

Auto-deployments appear in deployment history with a `webhook` trigger for auditing.

## Enable

1. Open the project
2. Go to the **Source** configuration tab
3. Toggle **Auto-Deploy** to ON

Requires a [GitHub-connected](./github.md) project (or equivalent webhook-capable source). Private GitHub App installs configure webhook delivery for you.

## Security

When Auto-Deploy is enabled, Docklift generates a **webhook secret**. Incoming requests must present a valid signature so arbitrary callers cannot trigger builds.

With the recommended private GitHub App flow, Docklift manages secret creation and verification automatically. Do not expose unsigned deploy endpoints.

## Behaviour notes

- Only pushes to the project's selected branch trigger a deploy
- Debounce: rapid successive pushes within ~10s coalesce into one build
- If a deployment is already running, wait or cancel it first — deployments are serialized per project
- Cancel mid-build tears containers down for a clean retry; past history entries are not rewritten

## Disable

Turn **Auto-Deploy** off on the Source tab. Manual **Deploy** / **Redeploy** still work from the dashboard.
