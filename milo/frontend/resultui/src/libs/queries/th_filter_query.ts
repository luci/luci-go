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

// TODO(weiweilin): figure out a clean way to dedupe with tr_search_query.ts

import { html } from 'lit-html';

import { Suggestion } from '../../components/auto_complete';
import { Variant } from '../../services/resultdb';
import { highlight } from '../lit_utils';
import { KV_SYNTAX_EXPLANATION, parseKeyValue } from './utils';

const VARIANT_FILTER_RE = /(-?)V:(.+)$/i;
const VARIANT_HASH_FILTER_RE = /(-?)VHASH:(.+)$/i;

export type VariantFilter = (v: Variant, hash: string) => boolean;

/**
 * Parses the variant predicate and variant filter from the query string. They
 * are useful for constructing RPC requests and filtering variant rows/groups.
 *
 * The returned VariantPredicate and VariantFilter may result in false
 * positives, so it should be used in conjunction of the TestVariantFilter
 * parsed from the same query.
 */
export function parseVariantFilter(filterQuery: string): VariantFilter {
  const filters: VariantFilter[] = [];
  for (const subQuery of filterQuery.split(' ')) {
    let match = subQuery.match(VARIANT_FILTER_RE);
    if (match) {
      const [, neg, value] = match;
      const [vKey, vValue] = parseKeyValue(value);

      const negate = neg === '-';
      if (vValue !== null) {
        filters.push((v) => negate !== (v.def[vKey] === vValue));
      } else {
        filters.push((v) => negate !== (v.def[vKey] !== undefined));
      }
      continue;
    }

    match = subQuery.match(VARIANT_HASH_FILTER_RE);
    if (match) {
      const [, neg, value] = match;
      const valueUpper = value.toUpperCase();
      const negate = neg === '-';
      filters.push((_, hash) => negate !== (hash.toUpperCase() === valueUpper));
    }
  }

  return (v, hash) => filters.every((f) => f(v, hash));
}

// Queries with arbitrary value.
const QUERY_TYPE_SUGGESTIONS = [
  {
    type: 'V:',
    explanation: `Include only tests with a matching variant key-value pair (${KV_SYNTAX_EXPLANATION})`,
  },
  {
    type: '-V:',
    explanation: `Exclude tests with a matching variant key-value pair (${KV_SYNTAX_EXPLANATION})`,
  },
  {
    type: 'VHash:',
    explanation: 'Include only tests with the specified variant hash (case insensitive)',
  },
  {
    type: '-VHash:',
    explanation: 'Exclude tests with the specified variant hash (case insensitive)',
  },
];

export function suggestTestHistoryFilterQuery(query: string): readonly Suggestion[] {
  if (query === '') {
    // Return some example queries when the query is empty.
    return [
      {
        isHeader: true,
        display: html`<strong>Advanced Syntax</strong>`,
      },
      {
        value: '-V:test_suite=browser_test',
        explanation: "Use '-' prefix to negate the filter",
      },
      {
        value: 'V:os=MacOS -V:test_suite=browser_test',
        explanation: 'Specify multiple variant filters.',
      },

      // Put this section behind `Advanced Syntax` so `Advanced Syntax` won't
      // be hidden after the size of supported filter types grows.
      {
        isHeader: true,
        display: html`<strong>Supported Filter Types</strong>`,
      },
      {
        value: 'V:uri-encoded-variant-key=uri-encoded-variant-value',
        explanation: 'Include only tests with a matching test variant key-value pair (case sensitive)',
      },
      {
        value: 'VHash:variant-hash',
        explanation: 'Include only tests with the specified variant hash (case insensitive)',
      },
    ];
  }

  const subQuery = query.split(' ').pop()!;
  if (subQuery === '') {
    return [];
  }

  const suggestions: Suggestion[] = [];

  // Suggest queries with arbitrary value.
  const match = subQuery.match(/^([^:]*:?)(.*)$/);
  if (match) {
    const [, subQueryType, subQueryValue] = match as [string, string, string];
    const typeUpper = subQueryType.toUpperCase();
    suggestions.push(
      ...QUERY_TYPE_SUGGESTIONS.flatMap(({ type, explanation }) => {
        if (type.toUpperCase().includes(typeUpper)) {
          return [{ value: type + subQueryValue, explanation }];
        }

        if (subQueryValue === '') {
          return [{ value: type + subQueryType, explanation }];
        }

        return [];
      })
    );
  }

  return suggestions.map((s) => ({ ...s, display: s.display || highlight(s.value!, subQuery) }));
}
