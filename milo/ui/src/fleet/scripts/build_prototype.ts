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
 * programmatic build API with clean virtual-module stubs (zero regex AST
 * rewriting of React components) and emits reproducible `--full-index` git
 * patches (`changes.patch`) + source trees (`source_tree/`).
 */

/* eslint-disable no-console */

import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import * as fs from 'node:fs';
import * as path from 'node:path';

export interface BuildPrototypeOptions {
  uiDir: string;
  outDir: string;
  slug: string;
  title: string;
  description: string;
  route: string;
}

export interface PrototypeMetaManifest {
  slug: string;
  title: string;
  description: string;
  defaultRoute: string;
  timestamp: string;
  baseCommitSha?: string;
  modifiedFiles?: string[];
}

export const FLEET_PATCH_EXCLUDED_PATHS = [
  'src/fleet/.agents',
  'src/fleet/Makefile',
  'src/fleet/docs/easy-mock-architecture.md',
  'src/fleet/scripts/build_prototype.ts',
  'src/fleet/scripts/build_prototype.test.ts',
];

export function isExcludedFromPrototypePatch(relPath: string): boolean {
  const norm = relPath.replace(/\\/g, '/').replace(/^\/+/, '');
  return FLEET_PATCH_EXCLUDED_PATHS.some(
    (ex) => norm === ex || norm.startsWith(ex + '/'),
  );
}

export function stripCrossOriginAttributes(html: string): string {
  return html
    .replace(
      /<meta\s+http-equiv=["']Content-Security-Policy["'][^>]*>\s*/gi,
      '',
    )
    .replace(/\s+crossorigin(?:="[^"]*")?/gi, '');
}

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
    title: options.title || cleanSlug,
    description:
      options.description || `Fleet Console static UI mock (${cleanSlug})`,
    defaultRoute: options.route || '/ui/fleet/p/chromeos/devices',
    timestamp,
  };
}

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
  const body =
    lines.map((line) => `+${line}`).join('\n') +
    (!normalizedContent.endsWith('\n') ? '\n\\ No newline at end of file' : '');
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

export function generatePrototypeSourceArtifacts(
  uiDir: string,
  outDir: string,
  meta: PrototypeMetaManifest,
): { baseCommitSha: string; modifiedFiles: string[]; patchContent: string } {
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
    ...FLEET_PATCH_EXCLUDED_PATHS.map((p) => `:(exclude)${p}`),
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
    '# Apply on current branch (from milo/ui): git apply --3way changes.patch',
    '',
  ].join('\n');
  const patchContent = combinedDiff
    ? `${patchHeader}${combinedDiff}\n`
    : `${patchHeader}# Clean ToT Fleet Console build (${baseCommitSha})\n`;

  fs.writeFileSync(path.join(outDir, 'changes.patch'), patchContent, 'utf-8');
  return { baseCommitSha, modifiedFiles, patchContent };
}

export function generateOfflineSettingsJs(
  defaultRoute = '/ui/fleet/p/chromeos/devices',
): string {
  const safeDefaultRoute = JSON.stringify(defaultRoute).replace(
    /</g,
    '\\u003c',
  );
  return `window.__FCON_EASY_MOCK__ = true;
(function() {
  var p = new URLSearchParams(location.search);
  var rawHash = location.hash ? location.hash.slice(1) : '';
  var hashRoute = (rawHash.indexOf('/ui/') === 0)
    ? rawHash
    : (rawHash.indexOf('ui/') === 0 ? '/' + rawHash : '');
  var r = hashRoute || p.get('route') || p.get('target_route') ||
    window.__FCON_INITIAL_ROUTE__ || ${safeDefaultRoute};
  var normalized = r
    .replace(/^\\/ui\\/fleet\\/labs\\/home\\b/, '/ui/fleet')
    .replace(/^\\/ui\\/fleet\\/labs\\/devices\\b/, '/ui/fleet/p/chromeos/devices')
    .replace(/^\\/ui\\/fleet\\/labs\\//, '/ui/fleet/');
  window.__FCON_INITIAL_ROUTE__ = normalized;
  try {
    var cleanUrl = new URL(location.href);
    cleanUrl.searchParams.delete('route');
    cleanUrl.searchParams.delete('target_route');
    cleanUrl.hash = '#' + normalized;
    if (location.href !== cleanUrl.toString()) {
      history.replaceState(null, '', cleanUrl.toString());
    }
  } catch (e) {}
})();
window.dataLayer = window.dataLayer || [];
window.gtag = window.gtag || function() { window.dataLayer.push(arguments); };
window.SETTINGS = window.SETTINGS || {
  firebase: { projectId: 'luci-milo-dev', appId: '1:0000:web:0000', apiKey: 'mock' },
  buildbucket: { host: 'cr-buildbucket.appspot.com' },
  swarming: { defaultHost: 'chromium-swarm.appspot.com', allowedHosts: ['chromium-swarm.appspot.com'] },
  resultdb: { host: 'results.api.cr.dev' },
  luciAnalysis: { host: 'analysis.api.cr.dev', uiHost: 'analysis.cr.dev' },
  luciBisection: { host: 'bisection.api.cr.dev' },
  sheriffOMatic: { host: 'sheriff-o-matic.appspot.com' },
  luciTreeStatus: { host: 'tree-status.api.cr.dev' },
  luciNotify: { host: 'notify.api.cr.dev' },
  authService: { host: 'chrome-infra-auth.appspot.com' },
  crRev: { host: 'crrev.com' },
  milo: { host: 'luci-milo.appspot.com', project: 'chromium', errorReportingApiKey: '' },
  luciSourceIndex: { host: 'source-index.api.cr.dev' },
  fleetConsole: { host: 'fleet-console.api.cr.dev', hats: {} },
  ufs: { host: 'ufs.api.cr.dev' },
  testInvestigate: { hatsPositiveRecs: {}, hatsNegativeRecs: {}, hatsCuj: {} }
};
`;
}

export function buildEasyMockAppModule(): string {
  return `import '@/common/styles/common_style.css';
import '@/common/styles/color_classes.css';
import '@/common/components/tooltip';

import { ThemeProvider } from '@emotion/react';
import { LocalizationProvider } from '@mui/x-date-pickers';
import { AdapterLuxon } from '@mui/x-date-pickers/AdapterLuxon';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { createMemoryRouter, redirect, RouterProvider } from 'react-router';

import releaseNotes from '@root/RELEASE_NOTES.md?raw';

import { obtainAuthState } from '@/common/api/auth_state';
import { AuthStateInitializer } from '@/common/components/auth_state_provider';
import { RecoverableErrorBoundary, RouteErrorDisplay } from '@/common/components/error_handling';
import { LitEnvProvider } from '@/common/components/lit_env_provider';
import { PageConfigStateProvider } from '@/common/components/page_config_state_provider';
import { PageMetaProvider } from '@/common/components/page_meta';
import { VersionControlProvider } from '@/common/components/version_control/version_control_provider';
import { FeatureFlagsProvider } from '@/common/feature_flags/provider';
import { BaseLayout } from '@/common/layouts/base_layout';
import { Store, StoreProvider } from '@/common/store';
import { theme } from '@/common/themes/base';
import { ReleaseNotesProvider } from '@/core/components/release_notes';
import { parseReleaseNotes } from '@/core/components/release_notes/common';
import { routes } from '@/core/routes';
import { FleetConsoleMockAPI } from '@/fleet/testing_tools/mock_api';
import { ReactLitBridge } from '@/generic_libs/components/react_lit_element';
import { SingletonStoreProvider } from '@/generic_libs/hooks/singleton';
import { SyncedSearchParamsProvider } from '@/generic_libs/hooks/synced_search_params';

export function App() {
  const [store] = useState(() => Store.create({}));
  const [queryClient] = useState(() => new QueryClient({ defaultOptions: { queries: { retry: false } } }));

  if (!FleetConsoleMockAPI.isPrototypeMode()) {
    FleetConsoleMockAPI.initPrototypeMode({
      authState: { email: 'user@google.com', identity: 'user:user@google.com' },
    });
  }

  const rawHash = window.location.hash ? window.location.hash.slice(1) : '';
  const initialMockRoute = rawHash.startsWith('/ui/')
    ? rawHash
    : (window as unknown as { __FCON_INITIAL_ROUTE__?: string }).__FCON_INITIAL_ROUTE__ ||
      '/ui/fleet/p/chromeos/devices';

  const [router] = useState(() =>
    createMemoryRouter(
      [
        { index: true, loader: () => redirect(initialMockRoute) },
        {
          path: 'ui',
          handle: { appId: 'milo' },
          loader: async () => obtainAuthState(),
          element: (
            <SyncedSearchParamsProvider>
              <AuthStateInitializer>
                <FeatureFlagsProvider>
                  <RecoverableErrorBoundary>
                    <ReactLitBridge>
                      <milo-tooltip />
                      <BaseLayout />
                    </ReactLitBridge>
                  </RecoverableErrorBoundary>
                </FeatureFlagsProvider>
              </AuthStateInitializer>
            </SyncedSearchParamsProvider>
          ),
          errorElement: <RouteErrorDisplay />,
          children: [...routes],
        },
        { path: '*', loader: () => redirect(initialMockRoute) },
      ],
      { initialEntries: [initialMockRoute] },
    ),
  );

  useEffect(() => {
    const unsub = router.subscribe((state) => {
      const nextHash = '#' + state.location.pathname + state.location.search + state.location.hash;
      if (window.location.hash !== nextHash) {
        const url = new URL(window.location.href);
        url.hash = nextHash;
        window.history.replaceState(null, '', url.toString());
      }
    });
    const onHash = () => {
      const h = window.location.hash ? window.location.hash.slice(1) : '';
      if (h.startsWith('/ui/')) router.navigate(h, { replace: true });
    };
    window.addEventListener('hashchange', onHash);
    window.addEventListener('popstate', onHash);
    return () => {
      unsub();
      window.removeEventListener('hashchange', onHash);
      window.removeEventListener('popstate', onHash);
    };
  }, [router]);

  return (
    <LocalizationProvider dateAdapter={AdapterLuxon}>
      <ThemeProvider theme={theme}>
        <QueryClientProvider client={queryClient}>
          <SingletonStoreProvider>
            <VersionControlProvider>
              <StoreProvider value={store}>
                <LitEnvProvider>
                  <PageMetaProvider>
                    <ReleaseNotesProvider initReleaseNotes={parseReleaseNotes(releaseNotes)}>
                      <PageConfigStateProvider>
                        <RouterProvider router={router} />
                      </PageConfigStateProvider>
                    </ReleaseNotesProvider>
                  </PageMetaProvider>
                </LitEnvProvider>
              </StoreProvider>
            </VersionControlProvider>
          </SingletonStoreProvider>
        </QueryClientProvider>
      </ThemeProvider>
    </LocalizationProvider>
  );
}
`;
}

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

  const easyMockPlugin = {
    name: 'fcon-easy-mock-static-builder',
    enforce: 'pre' as const,
    resolveId(id: string) {
      if (id === 'virtual:override-milo-host')
        return '\0virtual:override-milo-host';
      if (id === 'idb-keyval') return '\0virtual:idb-keyval-memory';
      return null;
    },
    load(id: string) {
      if (id === '\0virtual:override-milo-host') return 'export default {};\n';
      if (id === '\0virtual:idb-keyval-memory') {
        return [
          'const m = new Map();',
          'export const get = async (k) => m.get(k);',
          'export const set = async (k, v) => { m.set(k, v); };',
          'export const del = async (k) => { m.delete(k); };',
          'export const clear = async () => { m.clear(); };',
          'export const keys = async () => Array.from(m.keys());',
          'export const createStore = () => m;',
        ].join(' ');
      }
      const norm = id.replace(/\\/g, '/');
      if (norm.endsWith('/src/firebase.ts'))
        return 'export const db = {} as never;\n';
      if (norm.endsWith('/src/core/routes/routes.ts')) {
        return [
          "import { fleetRoutes } from '@/fleet/routes';",
          "export const routes = [{ path: 'fleet', handle: { appId: 'fleet' }, children: fleetRoutes }];",
        ].join('\n');
      }
      if (norm.endsWith('/src/App.tsx')) return buildEasyMockAppModule();
      return undefined;
    },
    transformIndexHtml(html: string) {
      const out = stripCrossOriginAttributes(html);
      const defaultRouteJson = JSON.stringify(
        options.route || '/ui/fleet/p/chromeos/devices',
      ).replace(/</g, '\\u003c');
      const boot = [
        `<script>window.__FCON_EASY_MOCK__ = true; window.__FCON_INITIAL_ROUTE__ = ${defaultRouteJson};</script>`,
        '<script src="./ui_version.js"></script>',
        '<script src="./settings.js"></script>',
      ].join('\n');
      return out.replace('<head>', `<head>\n${boot}`);
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
      react({
        babel: { configFile: true },
        exclude: [/node_modules/, /\/src\/proto\//, /\/src\/fleet\//],
      }),
      tsconfigPaths(),
    ],
  });

  fs.writeFileSync(
    path.join(outDir, 'ui_version.js'),
    'self.UI_VERSION = "easy-mock-tot";\nself.UI_VERSION_TYPE = "new-ui";\n',
    'utf-8',
  );
  fs.writeFileSync(
    path.join(outDir, 'settings.js'),
    generateOfflineSettingsJs(options.route),
    'utf-8',
  );

  const meta = buildPrototypeMetadata(options);
  const { baseCommitSha, modifiedFiles } = generatePrototypeSourceArtifacts(
    uiDir,
    outDir,
    meta,
  );
  meta.baseCommitSha = baseCommitSha;
  meta.modifiedFiles = modifiedFiles;
  fs.writeFileSync(
    path.join(outDir, '.prototype-meta.json'),
    JSON.stringify(meta, null, 2),
    'utf-8',
  );
  return meta;
}

export function parseCliArgs(
  argv: string[],
  defaultUiDir: string,
): BuildPrototypeOptions {
  const parsed: Record<string, string> = {};
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    if (arg.startsWith('--')) {
      const key = arg.slice(2);
      const next = argv[i + 1];
      if (next && !next.startsWith('--')) {
        parsed[key] = next;
        i++;
      } else {
        parsed[key] = 'true';
      }
    }
  }
  const outDir =
    parsed['out-dir'] ||
    process.env.FCON_EASY_MOCK_OUT_DIR ||
    path.join(defaultUiDir, '.tmp', 'dist_proto');
  return {
    uiDir: defaultUiDir,
    outDir: path.resolve(defaultUiDir, outDir),
    slug: parsed.slug || parsed.name || parsed.project || 'tot-fleet-console',
    title: parsed.title || 'Fleet Console Easy Mock',
    description:
      parsed.description || 'Standalone Fleet Console UI prototype bundle',
    route:
      parsed.route || parsed['entry-point'] || '/ui/fleet/p/chromeos/devices',
  };
}

const isDirectExecution = Boolean(
  process.argv[1] &&
    process.argv[1].endsWith('build_prototype.ts') &&
    !process.env.JEST_WORKER_ID,
);

if (isDirectExecution) {
  const scriptDir = path.dirname(path.resolve(process.argv[1]));
  const uiDir = path.resolve(scriptDir, '..', '..', '..');
  const opts = parseCliArgs(process.argv.slice(2), uiDir);
  runPrototypeBuild(opts)
    .then((meta) => {
      console.log(
        `[BUILD READY] Built Fleet Console static prototype (${meta.slug}) -> ${opts.outDir}`,
      );
    })
    .catch((err) => {
      console.error('Prototype build failed:', err);
      process.exit(1);
    });
}
