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

import { stripQuotes } from '@/fleet/utils/filters';

/**
 * Removes surrounding quotes from a string.
 */
export const unquote = (val: string) => stripQuotes(val);

/**
 * Normalizes a filter key by removing 'labels.' prefix and quotes.
 * E.g., 'labels."build"' -> 'build'
 */
export const normalizeFilterKey = (key: string) =>
  stripQuotes(key.replace(/^labels\./, ''));
