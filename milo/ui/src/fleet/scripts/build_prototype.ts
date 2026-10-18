#!/usr/bin/env -S node --experimental-strip-types
// Copyright 2026 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/**
 * @fileoverview Portable Static Prototype Bundle Builder for Fleet Console.
 *
 * Compiles a Fleet-only static UI bundle into `.tmp/dist_proto/` using Vite's
 * programmatic build API with `prototype_index.html`, `prototype_app.tsx`,
 * declarative virtual module stubs, and reproducible `--full-index` git patches
 * (`changes.patch`) + source trees (`source_tree/`).
 */

/* eslint-disable no-console */

import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import * as fs from 'node:fs';
import * as path from 'node:path';

/**
 * Options for building a standalone Fleet Console static prototype bundle.
 */
export interface BuildPrototypeOptions {
  readonly uiDir: string;
  readonly outDir: string;
  readonly slug: string;
  readonly title: string;
  readonly description: string;
  readonly route: string;
}

/**
 * Metadata written to `.prototype-meta.json` in the output bundle directory.
 */
export interface PrototypeMetaManifest {
  readonly slug: string;
  readonly title: string;
  readonly description: string;
  readonly defaultRoute: string;
  readonly timestamp: string;
  readonly baseCommitSha?: string;
  readonly modifiedFiles?: readonly string[];
}

/**
 * Git diff artifacts emitted alongside the compiled static bundle.
 */
export interface PrototypeSourceArtifacts {
  readonly baseCommitSha: string;
  readonly modifiedFiles: string[];
  readonly patchContent: string;
}

export const FLEET_PATCH_EXCLUDED_PATHS: readonly string[] = [
  'src/fleet/.agents',
  'src/fleet/Makefile',
  'src/fleet/docs/easy-mock-architecture.md',
  'src/fleet/scripts',
];

const DEFAULT_FLEET_ROUTE = '/ui/fleet/p/chromeos/devices';

/**
 * Bare-specifier virtual module stubs replacing external browser storage,
 * Firestore, and MobX runtime dependencies with lightweight in-memory mocks.
 */
const VIRTUAL_MODULE_STUBS: Readonly<Record<string, string>> = {
  'virtual:override-milo-host': 'export default {};\n',
  mobx: 'export const configure = () => {};\n',
  'idb-keyval': [
    'const store = new Map();',
    'export const get = async (key) => store.get(key);',
    'export const set = async (key, val) => { store.set(key, val); };',
    'export const del = async (key) => { store.delete(key); };',
    'export const clear = async () => { store.clear(); };',
    'export const keys = async () => Array.from(store.keys());',
    'export const createStore = () => store;',
  ].join('\n'),
  'firebase/firestore': [
    'export class Timestamp {',
    '  constructor(seconds = 0, nanoseconds = 0) {',
    '    this.seconds = seconds;',
    '    this.nanoseconds = nanoseconds;',
    '  }',
    '  toDate() { return new Date(this.seconds * 1000); }',
    '  toMillis() { return this.seconds * 1000; }',
    '  static now() { return new Timestamp(Math.floor(Date.now() / 1000), 0); }',
    '  static fromDate(date) {',
    '    return new Timestamp(Math.floor(date.getTime() / 1000), 0);',
    '  }',
    '}',
    'export const doc = () => ({});',
    'export const onSnapshot = () => () => {};',
  ].join('\n'),
};

/**
 * Repository path-suffix stubs replacing heavyweight or non-Fleet Milo modules.
 */
const REPO_MODULE_STUBS: Readonly<Record<string, string>> = {
  '/src/firebase.ts': 'export const db = {} as never;\n',
  '/src/common/api/error_reporting/register_handlers.ts': 'export {};\n',
  '/src/common/api/error_reporting/reporter.ts':
    'export const errorReporter = { report: () => {} };\n',
  '/src/common/components/auth_state_provider/index.ts':
    "export * from './context';\nexport * from './hooks';\n",
  '/src/common/tools/sanitize_html/sanitize_html.ts': [
    'export function sanitizeHTML(html: string, _opts?: unknown) {',
    '  return html as unknown as TrustedHTML;',
    '}',
    'export function initDefaultTrustedTypesPolicy() {}',
  ].join('\n'),
  '/src/common/store/store.ts':
    'export const useStore = () => ({ authState: { setValue() {} } });\n',
  '/src/common/layouts/base_layout.tsx':
    "export const SIDE_BAR_OPEN_CACHE_KEY = 'side-bar-open';\n",
  '/src/common/layouts/cookie_consent_bar/cookie_consent_bar.tsx':
    'export function CookieConsentBar() { return null; }\n',
  '/src/common/layouts/version_banner.tsx':
    'export function VersionBanner() { return null; }\n',
  '/src/fleet/layouts/fleet_helmet.tsx': [
    'export function FleetHelmet({ pageTitle }: { pageTitle: string }) {',
    "  return <title>{pageTitle ? `${pageTitle} | Fleet Console` : 'Fleet Console'}</title>;",
    '}',
  ].join('\n'),
  '/src/generic_libs/components/code_mirror_editor.tsx': [
    'export function CodeMirrorEditor({ value, className }: {',
    '  value: string;',
    '  className?: string;',
    '  initOptions?: unknown;',
    '  onInit?: unknown;',
    '  css?: unknown;',
    '}) {',
    '  return <pre className={className}>{value}</pre>;',
    '}',
  ].join('\n'),
};

/**
 * Returns true when `relPath` belongs to builder or documentation paths that
 * must be excluded from prototype `changes.patch` artifacts.
 */
export function isExcludedFromPrototypePatch(relPath: string): boolean {
  const normalized = relPath.replace(/\\/g, '/').replace(/^\/+/, '');
  return FLEET_PATCH_EXCLUDED_PATHS.some(
    (excluded) =>
      normalized === excluded || normalized.startsWith(`${excluded}/`),
  );
}

/**
 * Validates that `outDir` resolves inside `uiDir` and outside `uiDir/src` so
 * pre-build cleanup never deletes source files.
 */
export function validateOutDir(uiDir: string, outDir: string): void {
  const resolvedUiDir = path.resolve(uiDir);
  const resolvedOutDir = path.resolve(uiDir, outDir);
  const resolvedSrcDir = path.join(resolvedUiDir, 'src');
  const relFromUi = path.relative(resolvedUiDir, resolvedOutDir);
  const relFromSrc = path.relative(resolvedSrcDir, resolvedOutDir);

  if (
    resolvedOutDir === path.parse(resolvedOutDir).root ||
    !relFromUi ||
    relFromUi.startsWith('..') ||
    !relFromSrc ||
    !relFromSrc.startsWith('..')
  ) {
    throw new Error(
      `Refusing to clean unsafe --out-dir "${resolvedOutDir}". ` +
        'Choose a scratch directory inside milo/ui outside src/, such as .tmp/dist_proto.',
    );
  }
}

/**
 * Builds the normalized prototype metadata manifest from CLI options.
 */
export function buildPrototypeMetadata(
  options: Pick<
    BuildPrototypeOptions,
    'slug' | 'title' | 'description' | 'route'
  >,
  timestamp = new Date().toISOString(),
): PrototypeMetaManifest {
  const cleanSlug =
    options.slug
      .trim()
      .replace(/[^a-zA-Z0-9._-]+/g, '-')
      .replace(/^-+|-+$/g, '') || 'fleet-prototype';
  return {
    slug: cleanSlug,
    title: options.title.trim() || cleanSlug,
    description:
      options.description.trim() ||
      `Fleet Console static UI mock (${cleanSlug})`,
    defaultRoute: options.route.trim() || DEFAULT_FLEET_ROUTE,
    timestamp,
  };
}

/**
 * Renders `prototype_index.html` with `defaultRoute` injected as the fallback
 * initial route.
 */
export function buildPrototypeIndexHtml(
  defaultRoute = DEFAULT_FLEET_ROUTE,
  uiDir = process.cwd(),
): string {
  const templatePath = resolveScriptAsset('prototype_index.html', uiDir);
  const template = fs.readFileSync(templatePath, 'utf-8');
  const safeDefaultRoute = JSON.stringify(defaultRoute).replace(
    /</g,
    '\\u003c',
  );
  return template.replace('__FCON_DEFAULT_ROUTE__', safeDefaultRoute);
}

/**
 * Strips the `crossorigin` attribute that Vite's internal `vite:build-html`
 * plugin adds to emitted `<script>` and `<link>` tags so bundles load cleanly
 * from `file://` and static preview hosts without CORS origin errors.
 */
export function stripCrossOriginAttributes(html: string): string {
  return html.replace(/\s+crossorigin(?:="[^"]*")?/gi, '');
}

/**
 * Replaces ts-proto binary `encode` and `decode` wire methods with no-ops while
 * preserving `fromJSON` and `toJSON` for JSON pRPC mocking.
 */
export function stripProtoBinaryWireMethods(code: string): string {
  return code.replace(/=\s*\{\n  encode\([\s\S]*?\n\};/g, (block) =>
    block.replace(
      /^=\s*\{\n  encode\([\s\S]*?\n  fromJSON\(/,
      '= {\n  encode(): any { return null; },\n  decode(): any { return null; },\n  fromJSON(',
    ),
  );
}

/**
 * Replaces the 100-method `FleetClientImpl` class in `fleet.pb.ts` with a
 * dynamic Proxy-backed client so Rollup tree-shakes unreferenced UFS message
 * types while supporting any UFS RPC method at runtime.
 */
export function slimUfsFleetClient(code: string): string {
  const clsIdx = code.indexOf('export class FleetClientImpl');
  if (clsIdx === -1) return code;
  const rpcIdx = code.indexOf('interface Rpc', clsIdx);
  if (rpcIdx === -1) return code;
  const proxyClient = [
    'export class FleetClientImpl {',
    '  static readonly DEFAULT_SERVICE = FleetServiceName;',
    '  private readonly rpc: Rpc;',
    '  private readonly service: string;',
    '  constructor(rpc: Rpc, opts?: { service?: string }) {',
    '    this.service = opts?.service ?? FleetServiceName;',
    '    this.rpc = rpc;',
    '    return new Proxy(this, {',
    '      get: (target, prop: string | symbol) =>',
    "        typeof prop !== 'string' || prop in target || prop === 'then'",
    '          ? Reflect.get(target, prop)',
    '          : (req: unknown) => this.rpc.request(this.service, prop, req ?? {}),',
    '    });',
    '  }',
    '}',
    '',
  ].join('\n');
  return code.slice(0, clsIdx) + proxyClient + code.slice(rpcIdx);
}

/**
 * Formats an untracked file as a git-apply compatible `--full-index` unified
 * diff.
 */
export function formatUntrackedFileDiff(
  relPath: string,
  content: string,
): string {
  const normalizedRel = relPath.replace(/\\/g, '/').replace(/^\/+/, '');
  const normalizedContent = content.replace(/\r\n/g, '\n');
  const blobSha = createHash('sha1')
    .update(`blob ${Buffer.byteLength(normalizedContent, 'utf-8')}\0`)
    .update(normalizedContent, 'utf-8')
    .digest('hex');
  const lines = normalizedContent.split('\n');
  if (lines.length > 0 && lines[lines.length - 1] === '') {
    lines.pop();
  }
  const trailingMarker = normalizedContent.endsWith('\n')
    ? ''
    : '\n\\ No newline at end of file';
  const body = lines.map((line) => `+${line}`).join('\n') + trailingMarker;
  return [
    `diff --git a/${normalizedRel} b/${normalizedRel}`,
    'new file mode 100644',
    `index ${'0'.repeat(40)}..${blobSha}`,
    '--- /dev/null',
    `+++ b/${normalizedRel}`,
    `@@ -0,0 +1,${lines.length} @@`,
    body,
    '',
  ].join('\n');
}

/**
 * Generates `changes.patch` and copies modified Fleet files into `source_tree/`
 * inside `outDir`.
 */
export function generatePrototypeSourceArtifacts(
  uiDir: string,
  outDir: string,
  meta: PrototypeMetaManifest,
): PrototypeSourceArtifacts {
  let baseCommitSha = 'HEAD';
  try {
    baseCommitSha = execFileSync('git', ['merge-base', 'HEAD', 'origin/main'], {
      cwd: uiDir,
      encoding: 'utf-8',
      stdio: ['ignore', 'pipe', 'ignore'],
    }).trim();
  } catch {
    baseCommitSha = 'HEAD';
  }

  const patchPathspecs = [
    '--',
    'src/fleet',
    ...FLEET_PATCH_EXCLUDED_PATHS.map((excluded) => `:(exclude)${excluded}`),
  ];

  let trackedDiff = '';
  const modifiedSet = new Set<string>();
  try {
    trackedDiff = execFileSync(
      'git',
      ['diff', '--full-index', baseCommitSha, '--relative', ...patchPathspecs],
      { cwd: uiDir, encoding: 'utf-8', stdio: ['ignore', 'pipe', 'ignore'] },
    );
    const changedList = execFileSync(
      'git',
      [
        'diff',
        '--name-only',
        '--diff-filter=ACMR',
        baseCommitSha,
        '--relative',
        ...patchPathspecs,
      ],
      { cwd: uiDir, encoding: 'utf-8', stdio: ['ignore', 'pipe', 'ignore'] },
    );
    for (const line of changedList.split('\n')) {
      const trimmed = line.trim();
      if (trimmed && !isExcludedFromPrototypePatch(trimmed)) {
        modifiedSet.add(trimmed);
      }
    }
  } catch {
    trackedDiff = '';
  }

  let untrackedDiffs = '';
  try {
    const untrackedOut = execFileSync(
      'git',
      ['ls-files', '--others', '--exclude-standard', '--', 'src/fleet'],
      { cwd: uiDir, encoding: 'utf-8', stdio: ['ignore', 'pipe', 'ignore'] },
    );
    for (const line of untrackedOut.split('\n')) {
      const relPath = line.trim();
      if (!relPath || isExcludedFromPrototypePatch(relPath)) continue;
      const absPath = path.join(uiDir, relPath);
      if (fs.existsSync(absPath) && fs.statSync(absPath).isFile()) {
        modifiedSet.add(relPath);
        untrackedDiffs += formatUntrackedFileDiff(
          relPath,
          fs.readFileSync(absPath, 'utf-8'),
        );
      }
    }
  } catch {
    untrackedDiffs = '';
  }

  const modifiedFiles = Array.from(modifiedSet).sort();
  const sourceTreeDir = path.join(outDir, 'source_tree');
  for (const relPath of modifiedFiles) {
    const srcFile = path.join(uiDir, relPath);
    const dstFile = path.join(sourceTreeDir, relPath);
    if (fs.existsSync(srcFile) && fs.statSync(srcFile).isFile()) {
      fs.mkdirSync(path.dirname(dstFile), { recursive: true });
      fs.copyFileSync(srcFile, dstFile);
    }
  }

  const combinedDiff = [trackedDiff.trimEnd(), untrackedDiffs.trimEnd()]
    .filter(Boolean)
    .join('\n\n');
  const patchHeader = [
    `# Fleet Console Prototype Patch (${meta.slug})`,
    `# Base-Commit: ${baseCommitSha}`,
    `# Default-Route: ${meta.defaultRoute}`,
    '# Apply on current branch (from milo/ui): git apply --3way --directory=milo/ui changes.patch',
    '',
  ].join('\n');
  const patchContent = combinedDiff
    ? `${patchHeader}${combinedDiff}\n`
    : `${patchHeader}# Clean ToT Fleet Console build (${baseCommitSha})\n`;

  fs.writeFileSync(path.join(outDir, 'changes.patch'), patchContent, 'utf-8');
  return { baseCommitSha, modifiedFiles, patchContent };
}

/**
 * Compiles the standalone Fleet Console static prototype bundle and writes
 * `.prototype-meta.json`, `changes.patch`, and `source_tree/`.
 */
export async function runPrototypeBuild(
  options: BuildPrototypeOptions,
): Promise<PrototypeMetaManifest> {
  const { build } = await import('vite');
  const reactMod = await import('@vitejs/plugin-react');
  const tsconfigPathsMod = await import('vite-tsconfig-paths');
  const react = reactMod.default;
  const tsconfigPaths = tsconfigPathsMod.default;

  const { uiDir, outDir } = options;
  validateOutDir(uiDir, outDir);
  fs.rmSync(outDir, { recursive: true, force: true });
  fs.mkdirSync(outDir, { recursive: true });

  const defaultRoute = options.route.trim() || DEFAULT_FLEET_ROUTE;
  const prototypeAppPath = resolveScriptAsset('prototype_app.tsx', uiDir);

  const easyMockPlugin = {
    name: 'fcon-easy-mock-static-builder',
    enforce: 'pre' as const,
    resolveId(id: string): string | null {
      if (id in VIRTUAL_MODULE_STUBS) return `\0virtual:${id}`;
      return null;
    },
    load(id: string): string | undefined {
      if (id.startsWith('\0virtual:')) {
        return VIRTUAL_MODULE_STUBS[id.slice('\0virtual:'.length)];
      }
      const norm = id.replace(/\\/g, '/');
      for (const [suffix, stubCode] of Object.entries(REPO_MODULE_STUBS)) {
        if (norm.endsWith(suffix)) return stubCode;
      }
      if (norm.endsWith('/src/App.tsx')) {
        return fs.readFileSync(prototypeAppPath, 'utf-8');
      }
      if (norm.includes('/src/proto/') && norm.endsWith('.pb.ts')) {
        const rawProto = fs.readFileSync(id, 'utf-8');
        const stripped = stripProtoBinaryWireMethods(rawProto);
        return norm.endsWith('/unifiedfleet/api/v1/rpc/fleet.pb.ts')
          ? slimUfsFleetClient(stripped)
          : stripped;
      }
      return undefined;
    },
    transformIndexHtml: {
      order: 'pre' as const,
      handler: (): string => buildPrototypeIndexHtml(defaultRoute, uiDir),
    },
  };

  const stripCrossOriginPlugin = {
    name: 'fcon-easy-mock-strip-crossorigin',
    transformIndexHtml: {
      order: 'post' as const,
      handler: (html: string): string => stripCrossOriginAttributes(html),
    },
  };

  await build({
    root: uiDir,
    configFile: false,
    base: './',
    clearScreen: false,
    logLevel: 'warn',
    define: {
      'process.env.BABEL_8_BREAKING': 'false',
      'process.env.BABEL_TYPES_8_BREAKING': 'false',
    },
    resolve: {
      alias: {
        '@': path.resolve(uiDir, 'src'),
        '@root': path.resolve(uiDir, './'),
        lodash: path.resolve(uiDir, 'node_modules/lodash-es'),
      },
    },
    assetsInclude: ['RELEASE_NOTES.md'],
    build: {
      outDir,
      assetsDir: 'assets',
      sourcemap: false,
      emptyOutDir: true,
      modulePreload: false,
      chunkSizeWarningLimit: 10000,
      rollupOptions: { output: { inlineDynamicImports: true } },
    },
    plugins: [
      easyMockPlugin,
      stripCrossOriginPlugin,
      react({
        babel: { configFile: true },
        exclude: [/node_modules/, /\/src\/proto\//, /\/src\/fleet\//],
      }),
      tsconfigPaths(),
    ],
  });

  const baseMeta = buildPrototypeMetadata(options);
  const { baseCommitSha, modifiedFiles } = generatePrototypeSourceArtifacts(
    uiDir,
    outDir,
    baseMeta,
  );
  const finalMeta: PrototypeMetaManifest = {
    ...baseMeta,
    baseCommitSha,
    modifiedFiles,
  };
  fs.writeFileSync(
    path.join(outDir, '.prototype-meta.json'),
    JSON.stringify(finalMeta, null, 2),
    'utf-8',
  );
  return finalMeta;
}

/**
 * Parses CLI flags into `BuildPrototypeOptions`.
 */
export function parseCliArgs(
  argv: readonly string[],
  defaultUiDir: string,
): BuildPrototypeOptions {
  const parsed: Record<string, string> = {};
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    if (!arg.startsWith('--')) continue;
    const key = arg.slice(2);
    const next = argv[i + 1];
    if (next && !next.startsWith('--')) {
      parsed[key] = next;
      i++;
    } else {
      parsed[key] = 'true';
    }
  }
  const outDir =
    parsed['out-dir'] ??
    process.env.FCON_EASY_MOCK_OUT_DIR ??
    path.join(defaultUiDir, '.tmp', 'dist_proto');
  return {
    uiDir: defaultUiDir,
    outDir: path.resolve(defaultUiDir, outDir),
    slug: parsed.slug ?? parsed.name ?? parsed.project ?? 'tot-fleet-console',
    title: parsed.title ?? 'Fleet Console Easy Mock',
    description:
      parsed.description ?? 'Standalone Fleet Console UI prototype bundle',
    route: parsed.route ?? parsed['entry-point'] ?? DEFAULT_FLEET_ROUTE,
  };
}

function resolveScriptAsset(fileName: string, uiDir: string): string {
  const fromUiDir = path.resolve(uiDir, 'src/fleet/scripts', fileName);
  if (fs.existsSync(fromUiDir)) {
    return fromUiDir;
  }
  return path.resolve(uiDir, 'milo/ui/src/fleet/scripts', fileName);
}

async function main(): Promise<void> {
  let uiDir = process.cwd();
  if (!fs.existsSync(path.join(uiDir, 'package.json'))) {
    const scriptDir = path.dirname(path.resolve(process.argv[1]));
    uiDir = path.resolve(scriptDir, '..', '..', '..');
  }
  const opts = parseCliArgs(process.argv.slice(2), uiDir);
  try {
    const meta = await runPrototypeBuild(opts);
    console.log(
      `[BUILD READY] Built Fleet Console static prototype (${meta.slug}) -> ${opts.outDir}`,
    );
  } catch (err) {
    console.error('Prototype build failed:', err);
    process.exit(1);
  }
}

const isDirectExecution = Boolean(
  process.argv[1] &&
    /\bbuild_prototype\.(?:ts|mjs|js)$/.test(process.argv[1]) &&
    !process.env.JEST_WORKER_ID,
);

if (isDirectExecution) {
  void main();
}
