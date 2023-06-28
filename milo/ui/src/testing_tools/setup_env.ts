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

import crypto from 'crypto';
import { TextDecoder, TextEncoder } from 'util';

import 'isomorphic-fetch';
import 'intersection-observer';
import { jest } from '@jest/globals';
import * as dotenv from 'dotenv';
import * as idbKeyVal from 'idb-keyval';
import { configure } from 'mobx';

import { assertNonNullable } from '@/generic_libs/tools/utils';
import './jest_utils';

// TODO(crbug/1347294): encloses all state modifying actions in mobx actions
// then delete this.
configure({ enforceActions: 'never' });

dotenv.config({
  path: './.env.development',
});

self.CONFIGS = Object.freeze({
  VERSION: assertNonNullable(process.env['VITE_MILO_VERSION']),
  RESULT_DB: {
    HOST: assertNonNullable(process.env['VITE_RESULT_DB_HOST']),
  },
  BUILDBUCKET: {
    HOST: assertNonNullable(process.env['VITE_BUILDBUCKET_HOST']),
  },
  LUCI_ANALYSIS: {
    HOST: assertNonNullable(process.env['VITE_LUCI_ANALYSIS_HOST']),
  },
  LUCI_BISECTION: {
    HOST: assertNonNullable(process.env['VITE_LUCI_BISECTION_HOST']),
  },
});
window.ENABLE_GA = false;

self.TextEncoder = TextEncoder;
self.TextDecoder = TextDecoder as typeof self.TextDecoder;

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

self.console.error = () => {
  // Silence errors printed out to the console during test.
};
