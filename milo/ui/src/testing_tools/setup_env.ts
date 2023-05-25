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

import 'intersection-observer';
import 'isomorphic-fetch';
import { jest } from '@jest/globals';
import crypto from 'crypto';
import * as dotenv from 'dotenv';
import * as idbKeyVal from 'idb-keyval';
import { configure } from 'mobx';
import { TextDecoder, TextEncoder } from 'util';

// TODO(crbug/1347294): encloses all state modifying actions in mobx actions
// then delete this.
configure({ enforceActions: 'never' });

dotenv.config({
  path: './.env.development',
});

self.CONFIGS = Object.freeze({
  RESULT_DB: {
    HOST: process.env['VITE_RESULT_DB_HOST']!,
  },
  BUILDBUCKET: {
    HOST: process.env['VITE_BUILDBUCKET_HOST']!,
  },
  LUCI_ANALYSIS: {
    HOST: process.env['VITE_LUCI_ANALYSIS_HOST']!,
  },
});
window.ENABLE_GA = false;

self.TextEncoder = TextEncoder;
self.TextDecoder = TextDecoder as typeof self.TextDecoder;

self.CSSStyleSheet.prototype.replace = () => Promise.race([]);

jest.mock('idb-keyval');
const idbMockStore = new Map();
(idbKeyVal.get as jest.Mock<typeof idbKeyVal.get>).mockImplementation(async (k) => {
  return idbMockStore.get(k);
});
(idbKeyVal.set as jest.Mock<typeof idbKeyVal.set>).mockImplementation(async (k, v) => {
  idbMockStore.set(k, v);
});

Object.defineProperty(self, 'crypto', {
  value: {
    subtle: crypto.webcrypto.subtle,
  },
});

// Silence errors printed out to the console during test.
self.console.error = () => {};
