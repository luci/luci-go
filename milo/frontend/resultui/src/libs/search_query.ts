// Copyright 2021 The LUCI Authors.
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

import { TestVariant } from '../services/resultdb';

const SPECIAL_QUERY_RE = /^(?<neg>-?)(?<type>[a-zA-Z]+):(?<value>.+)$/;

export type TestVariantFilter = (v: TestVariant) => boolean;

export function parseSearchQuery(searchQuery: string): TestVariantFilter {
  const filters = searchQuery.toUpperCase().split(' ').map((query) => {
    const match = query.match(SPECIAL_QUERY_RE);

    // If the query isn't a special query, treat it as an ID query.
    const {neg, type, value} = match?.groups! || {neg: '', type: 'ID', value: query};
    const negate = neg === '-';
    switch (type) {
      // Whether there's at least one a test result of the specified status.
      case 'RSTATUS':
        const statuses = value.split(',');
        return (v: TestVariant) => negate !== (v.results || []).some((r) => statuses.includes(r.result.status));
      // Whether the test ID contains the query as a substring (case insensitive).
      case 'ID':
        return (v: TestVariant) => negate !== v.testId.toUpperCase().includes(value);
      default:
        throw new Error(`invalid query type: ${type}`);
    }
  });
  return (v) => filters.every((f) => f(v));
}
