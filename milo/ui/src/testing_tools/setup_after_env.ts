// Copyright 2023 The LUCI Authors.
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

import '@testing-library/jest-dom';
import 'isomorphic-fetch';
import 'intersection-observer';
import '@/proto_utils/duration_patch';

import crypto from 'crypto';
import { TextDecoder, TextEncoder } from 'util';

import { matchers as emotionMatchers } from '@emotion/jest';
import * as dotenv from 'dotenv';
import * as idbKeyVal from 'idb-keyval';
import { configure } from 'mobx';
import ResizeObserver from 'resize-observer-polyfill';

import { assertNonNullable } from '@/generic_libs/tools/utils';

import {
  createSelectiveMockFromModule,
  createSelectiveSpiesFromModule,
} from './jest_utils';

expect.extend(emotionMatchers);

// TODO(crbug/1347294): encloses all state modifying actions in mobx actions
// then delete this.
configure({ enforceActions: 'never' });

dotenv.config({
  path: './.env.development',
});

// Those variables are declared as `const` so their value won't be accidentally
// changed. But they are actually injected by `/settings.js` and
// `/ui_version.js` to `self` in the production environment. Here we need to do
// the same so they are available to code run under the test environment.
const serverInjectedVars = self as unknown as {
  UI_VERSION: typeof UI_VERSION;
  UI_VERSION_TYPE: typeof UI_VERSION_TYPE;
  SETTINGS: typeof SETTINGS;
};
serverInjectedVars.UI_VERSION = assertNonNullable(
  process.env['VITE_LOCAL_UI_VERSION'],
);
serverInjectedVars.UI_VERSION_TYPE = 'new-ui';
serverInjectedVars.SETTINGS = Object.freeze({
  buildbucket: {
    host: assertNonNullable(process.env['VITE_BUILDBUCKET_HOST']),
  },
  swarming: {
    defaultHost: assertNonNullable(process.env['VITE_SWARMING_DEFAULT_HOST']),
    allowedHosts: assertNonNullable(
      process.env['VITE_SWARMING_ALLOWED_HOSTS'],
    ).split(';'),
  },
  resultdb: {
    host: assertNonNullable(process.env['VITE_RESULT_DB_HOST']),
  },
  luciAnalysis: {
    host: assertNonNullable(process.env['VITE_LUCI_ANALYSIS_HOST']),
    uiHost: assertNonNullable(process.env['VITE_LUCI_ANALYSIS_UI_HOST']),
  },
  luciBisection: {
    host: assertNonNullable(process.env['VITE_LUCI_BISECTION_HOST']),
  },
  sheriffOMatic: {
    host: assertNonNullable(process.env['VITE_SHERIFF_O_MATIC_HOST']),
  },
  luciTreeStatus: {
    host: assertNonNullable(process.env['VITE_TREE_STATUS_HOST']),
  },
  luciNotify: {
    host: assertNonNullable(process.env['VITE_LUCI_NOTIFY_HOST']),
  },
  authService: {
    host: assertNonNullable(process.env['VITE_AUTH_SERVICE_HOST']),
  },
  crRev: {
    host: assertNonNullable(process.env['VITE_CR_REV_HOST']),
  },
  milo: {
    host: assertNonNullable(process.env['VITE_MILO_HOST']),
  },
  luciSourceIndex: {
    host: assertNonNullable(process.env['VITE_LUCI_SOURCE_INDEX_HOST']),
  },
  fleetConsole: {
    host: assertNonNullable(process.env['VITE_FLEET_CONSOLE_HOST']),
    hats: {
      apiKey: assertNonNullable(process.env['VITE_FLEET_CONSOLE_HATS_API_KEY']),
      triggerId: assertNonNullable(
        process.env['VITE_FLEET_CONSOLE_HATS_TRIGGER_ID'],
      ),
      productId: Number(
        assertNonNullable(process.env['VITE_FLEET_CONSOLE_HATS_PRODUCT_ID']),
      ),
    },
    enableColumnFilter:
      process.env['VITE_FLEET_CONSOLE_ENABLE_COLUMN_FILTER'] === 'true',
  },
  ufs: {
    host: assertNonNullable(process.env['VITE_UFS_HOST']),
  },
  testInvestigate: {
    hatsPositiveRecs: {
      apiKey: assertNonNullable(process.env['VITE_INVESTIGATE_HATS_API_KEY']),
      triggerId: assertNonNullable(
        process.env['VITE_INVESTIGATE_HATS_POSITIVE_RECS_TRIGGER_ID'],
      ),
      productId: Number(
        assertNonNullable(process.env['VITE_INVESTIGATE_HATS_PRODUCT_ID']),
      ),
    },
    hatsNegativeRecs: {
      apiKey: assertNonNullable(process.env['VITE_INVESTIGATE_HATS_API_KEY']),
      triggerId: assertNonNullable(
        process.env['VITE_INVESTIGATE_HATS_NEGATIVE_RECS_TRIGGER_ID'],
      ),
      productId: Number(
        assertNonNullable(process.env['VITE_INVESTIGATE_HATS_PRODUCT_ID']),
      ),
    },
    hatsCuj: {
      apiKey: assertNonNullable(process.env['VITE_INVESTIGATE_HATS_API_KEY']),
      triggerId: assertNonNullable(
        process.env['VITE_INVESTIGATE_HATS_CUJ_TRIGGER_ID'],
      ),
      productId: Number(
        assertNonNullable(process.env['VITE_INVESTIGATE_HATS_PRODUCT_ID']),
      ),
    },
  },
});

// `jest.mock` calls are automatically moved to the beginning of a test file by
// the jest test runner (i.e. before any import statements), making it
// impossible to use imported symbols in the module factory.
//
// Make the following functions accessible through `self` so they can be used in
// the module factory in a
// `jest.mock('module-name', () => { /* module factory */ })` call.
self.createSelectiveMockFromModule = createSelectiveMockFromModule;
self.createSelectiveSpiesFromModule = createSelectiveSpiesFromModule;

self.TextEncoder = TextEncoder;
self.TextDecoder = TextDecoder as typeof self.TextDecoder;

self.ResizeObserver = ResizeObserver;

self.CSSStyleSheet.prototype.replace = () => Promise.race([]);

jest.mock('idb-keyval');
const idbMockStore = new Map();
jest.mocked(idbKeyVal.get).mockImplementation(async (k) => {
  return idbMockStore.get(k);
});
jest.mocked(idbKeyVal.set).mockImplementation(async (k, v) => {
  idbMockStore.set(k, v);
});

Object.defineProperty(self, 'crypto', {
  value: {
    subtle: crypto.webcrypto.subtle,
    // GetRandomValues is required by the nanoid package to run tests.
    getRandomValues: (arr: unknown[]) => crypto.randomBytes(arr.length),
  },
});

// jsdom does not support `scrollIntoView`.
// See https://github.com/jsdom/jsdom/issues/1695
window.HTMLElement.prototype.scrollIntoView = jest.fn();

jest.mock('lit/decorators.js', () => ({
  ...jest.requireActual('lit/decorators.js'),
  customElement(name: string) {
    return function (eleCon: CustomElementConstructor) {
      // jest's module mocking may cause the module to be initialized multiple
      // times and causing the element to be registered multiple times, leading
      // to error: 'NotSupportedError: This name has already been registered in
      // the registry.'
      //
      // Register the element conditionally to avoid the error.
      if (!customElements.get(name)) {
        customElements.define(name, eleCon);
      }
    };
  },
}));

// Install a noop gtag to avoid `undefined` issue during unit tests.
self.gtag = () => {};
