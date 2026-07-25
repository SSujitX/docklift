import fs from 'fs';
import path from 'path';
import crypto from 'crypto';
import { spawn } from 'child_process';
import type { ResolvedBuildService } from './buildResolver.js';

const RAILPACK_VERSION = '0.33.0';
const RAILPACK_FRONTEND = `ghcr.io/railwayapp/railpack-frontend:v${RAILPACK_VERSION}`;
const PROTECTED_BUILD_ENV = /^(?:PATH|HOME|PORT|NODE_OPTIONS|LD_PRELOAD|LD_LIBRARY_PATH|DOCKER_.+|BUILDKIT_.+)$/i;

interface BuildEnv {
  key: string;
  value: string;
  is_build_arg: boolean | null;
  is_secret?: boolean | null;
}

export function summarizeBuildFailure(command: string, output: string, code: number): string {
  if (
    /npm ci.*can only install packages|package\.json and package-lock\.json.*in sync/is.test(
      output
    )
  ) {
    return 'package.json and package-lock.json are out of sync. Run "npm install" in the project, commit the updated package-lock.json, then redeploy.';
  }
  if (/frozen lockfile|lockfile.*outdated|lockfile.*not.*up.to.date/is.test(output)) {
    return 'The dependency lockfile is out of sync with the project manifest. Regenerate and commit the lockfile with the project package manager, then redeploy.';
  }
  return `${command} exited with code ${code}`;
}

function run(
  command: string,
  args: string[],
  opts: { cwd?: string; env?: NodeJS.ProcessEnv; onProcess?: (child: ReturnType<typeof spawn>) => void },
  writeLog: (text: string) => void
): Promise<void> {
  return new Promise((resolve, reject) => {
    let output = '';
    const capture = (chunk: unknown) => {
      const text = String(chunk);
      output = `${output}${text}`.slice(-128 * 1024);
      writeLog(text);
    };
    const child = spawn(command, args, {
      cwd: opts.cwd,
      env: opts.env || process.env,
      shell: false,
    });
    opts.onProcess?.(child);
    child.stdout.on('data', capture);
    child.stderr.on('data', capture);
    child.on('error', reject);
    child.on('close', (code) => {
      if (code === 0) resolve();
      else {
        const exitCode = code ?? 1;
        reject(new Error(summarizeBuildFailure(command, output, exitCode)));
      }
    });
  });
}

export async function buildServiceImage(opts: {
  projectPath: string;
  statePath: string;
  service: ResolvedBuildService;
  imageTag: string;
  envVars: BuildEnv[];
  writeLog: (text: string) => void;
  onProcess?: (child: ReturnType<typeof spawn>) => void;
}): Promise<void> {
  const { projectPath, statePath, service, imageTag, envVars, writeLog, onProcess } = opts;
  const context = path.resolve(projectPath, service.contextPath);
  const invalidEnv = envVars.find((item) => !/^[A-Za-z_][A-Za-z0-9_]*$/.test(item.key));
  if (invalidEnv) {
    throw new Error(`Invalid environment variable name: ${invalidEnv.key}`);
  }
  const protectedEnv = envVars.find(
    (item) => item.is_build_arg && PROTECTED_BUILD_ENV.test(item.key)
  );
  if (protectedEnv) {
    throw new Error(
      `Build variable ${protectedEnv.key} is reserved by DockLift and cannot be overridden`
    );
  }

  if (service.builder === 'dockerfile') {
    if (!service.dockerfilePath) throw new Error('Resolved Dockerfile path is missing');
    const dockerfile = path.resolve(projectPath, service.dockerfilePath);
    const buildArgs = envVars.filter((item) => item.is_build_arg && !item.is_secret);
    const secrets = envVars.filter((item) => item.is_build_arg && item.is_secret);
    // Prefer BuildKit so --secret works when secrets are configured
    const args = secrets.length
      ? ['buildx', 'build', '--load', '--progress', 'plain', '-t', imageTag, '-f', dockerfile]
      : ['build', '--progress', 'plain', '-t', imageTag, '-f', dockerfile];
    for (const item of buildArgs) {
      args.push('--build-arg', `${item.key}=${item.value}`);
    }
    for (const item of secrets) {
      args.push('--secret', `id=${item.key},env=${item.key}`);
      writeLog(`🔐 Passing ${item.key} as BuildKit secret (not build-arg)\n`);
    }
    args.push(context);
    writeLog(`🐳 Building ${service.name} with ${service.dockerfilePath}\n`);
    const buildEnv = secrets.length
      ? {
          ...process.env,
          ...Object.fromEntries(secrets.map((item) => [item.key, item.value])),
        }
      : process.env;
    await run('docker', args, { cwd: context, env: buildEnv, onProcess }, writeLog);
    return;
  }

  fs.mkdirSync(statePath, { recursive: true });
  const safeName = service.name.replace(/[^a-zA-Z0-9_-]/g, '-');
  const planPath = path.join(statePath, `${safeName}-railpack-plan.json`);
  const infoPath = path.join(statePath, `${safeName}-railpack-info.json`);
  const buildVars = envVars.filter((item) => item.is_build_arg);
  const prepareArgs = [
    'prepare',
    context,
    '--plan-out',
    planPath,
    '--info-out',
    infoPath,
    '--env',
    `PORT=${service.internalPort}`,
  ];
  // Railpack needs names during planning, but actual values are supplied to BuildKit
  // as secrets. A fixed placeholder keeps secrets out of argv and the plan.
  for (const item of buildVars) {
    prepareArgs.push('--env', `${item.key}=__DOCKLIFT_BUILD_SECRET__`);
  }

  writeLog(`🛤️  No Dockerfile found → analyzing ${service.contextPath} with Railpack\n`);
  await run('railpack', prepareArgs, { cwd: context, onProcess }, writeLog);

  const buildArgs = [
    'buildx',
    'build',
    '--load',
    '--progress',
    'plain',
    '--build-arg',
    `BUILDKIT_SYNTAX=${RAILPACK_FRONTEND}`,
    '--build-arg',
    `PORT=${service.internalPort}`,
    '--secret',
    'id=PORT,env=PORT',
    '-f',
    planPath,
    '-t',
    imageTag,
  ];
  if (buildVars.length) {
    const secretsHash = crypto
      .createHash('sha256')
      .update(
        buildVars
          .map((item) => `${item.key}=${item.value}`)
          .sort()
          .join('\n')
      )
      .digest('hex');
    buildArgs.push('--build-arg', `secrets-hash=${secretsHash}`);
  }
  for (const item of buildVars) {
    buildArgs.push('--secret', `id=${item.key},env=${item.key}`);
  }
  buildArgs.push(context);
  const buildEnv = {
    ...process.env,
    PORT: String(service.internalPort),
    ...Object.fromEntries(buildVars.map((item) => [item.key, item.value])),
  };
  writeLog(`📦 Railpack plan ready; building ${imageTag}\n`);
  await run('docker', buildArgs, { cwd: context, env: buildEnv, onProcess }, writeLog);
}
