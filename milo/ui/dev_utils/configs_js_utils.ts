// Copyright 2024 The LUCI Authors.
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

import * as path from 'node:path';

import { Plugin } from 'vite';

import { assertNonNullable } from '../src/generic_libs/tools/utils';

/**
 * Construct a `configs.js` file from environment variables.
 */
export function getLocalDevConfigsJs(env: Record<string, string | undefined>) {
  const localVersion = env['VITE_MILO_VERSION'];
  const localSettings: typeof SETTINGS = {
    buildbucket: {
      host: assertNonNullable(env['VITE_BUILDBUCKET_HOST']),
    },
    swarming: {
      defaultHost: assertNonNullable(env['VITE_SWARMING_DEFAULT_HOST']),
    },
    resultdb: {
      host: assertNonNullable(env['VITE_RESULT_DB_HOST']),
    },
    luciAnalysis: {
      host: assertNonNullable(env['VITE_LUCI_ANALYSIS_HOST']),
      uiHost: assertNonNullable(env['VITE_LUCI_ANALYSIS_UI_HOST']),
    },
    luciBisection: {
      host: assertNonNullable(env['VITE_LUCI_BISECTION_HOST']),
    },
    luciNotify: {
      host: assertNonNullable(env['VITE_LUCI_NOTIFY_HOST']),
    },
    sheriffOMatic: {
      host: assertNonNullable(env['VITE_SHERIFF_O_MATIC_HOST']),
    },
    luciTreeStatus: {
      host: assertNonNullable(env['VITE_TREE_STATUS_HOST']),
    },
    authService: {
      host: assertNonNullable(env['VITE_AUTH_SERVICE_HOST']),
    },
    crRev: {
      host: assertNonNullable(env['VITE_CR_REV_HOST']),
    },
    milo: {
      host: assertNonNullable(env['VITE_MILO_HOST']),
    },
    luciSourceIndex: {
      host: assertNonNullable(env['VITE_LUCI_SOURCE_INDEX_HOST']),
    },
  };

  const localDevConfigsJs =
    `self.VERSION = '${localVersion}';\n` +
    `self.SETTINGS = Object.freeze(${JSON.stringify(
      localSettings,
      undefined,
      2,
    )});\n`;

  return localDevConfigsJs;
}

/**
 * Get a virtual-configs-js plugin so we can import configs.js in the service
 * workers with the correct syntax required by different environments.
 */
export function getVirtualConfigsJsPlugin(
  mode: string,
  env: Record<string, string | undefined>,
): Plugin {
  return {
    name: 'virtual-configs-js',
    resolveId: (id, importer) => {
      if (id !== 'virtual:configs.js') {
        return null;
      }

      // `importScripts` is only available in workers.
      // Ensure this module is only used by service workers.
      if (
        !['src/sw/root_sw.ts', 'src/sw/ui_sw.ts']
          .map((p) => path.join(__dirname, '../', p))
          .includes(importer || '')
      ) {
        throw new Error(
          'virtual:configs.js should only be imported by a service worker script.',
        );
      }
      return '\0virtual:configs.js';
    },
    load: (id) => {
      if (id !== '\0virtual:configs.js') {
        return null;
      }

      // In production, the service worker script cannot be a JS module due to
      // limited browser support. So we need to use `importScripts` instead of
      // `import` to load `/configs.js`.
      if (mode !== 'development') {
        return "importScripts('/configs.js');";
      }

      // During development, the service worker script can only be a JS module,
      // because it runs through the same pipeline as the rest of the scripts.
      // It cannot use the `importScripts`. So we inject the configs directly.
      return getLocalDevConfigsJs(env);
    },
  };
}

/**
 * Get a override-milo-host plugin so we can override Milo (API) host specified
 * in configs.js if required.
 *
 * This is useful when deploying an UI only demo where the API service of the
 * same version does not exist.
 */
export function overrideMiloHostPlugin(
  env: Record<string, string | undefined>,
): Plugin {
  const miloHost = env['VITE_OVERRIDE_MILO_HOST'];
  return {
    name: 'override-milo-host',
    resolveId: (id) => {
      if (id !== 'virtual:override-milo-host') {
        return null;
      }

      return '\0virtual:override-milo-host';
    },
    load: (id) => {
      if (id !== '\0virtual:override-milo-host') {
        return null;
      }

      if (!miloHost) {
        return '';
      }

      return `
        self.SETTINGS = Object.freeze({
          ...self.SETTINGS,
          milo: {
            ...self.SETTINGS.milo,
            host: ${JSON.stringify(miloHost)}
          }
        });
      `;
    },
  };
}
