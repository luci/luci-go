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
import { Variant, VariantPredicate } from '../../services/resultdb';
import { TestVariantHistoryEntry } from '../../services/test_history_service';
import { highlight } from '../utils';

const FILTER_QUERY_RE = /^(-?)([a-zA-Z]+):(.+)$/;

export type TVHEntryFilter = (v: TestVariantHistoryEntry) => boolean;

export function parseTVHFilterQuery(filterQuery: string): TVHEntryFilter {
  const filters = filterQuery
    .split(' ')
    .filter((s) => s !== '')
    .map((query) => {
      const match = query.match(FILTER_QUERY_RE);
      if (!match) {
        throw new Error(`invalid query. the query should have the format of ${FILTER_QUERY_RE}`);
      }

      const [, neg, queryType, value] = match;
      const valueUpper = value.toUpperCase();
      const negate = neg === '-';
      switch (queryType.toUpperCase()) {
        // Whether the test variant has the specified status.
        case 'STATUS': {
          const statuses = valueUpper.split(',');
          return (v: TestVariantHistoryEntry) => negate !== statuses.includes(v.status);
        }
        // Whether the test variant has a matching variant key-value pair.
        case 'V': {
          // Don't use String.split here because then vValue can't contain '='.
          const [, vKey, vValue] = value.match(/^([^=]*)(?:=(.*))?$/)!;

          // If the variant value is unspecified, accept any value.
          // Otherwise, the value must match the specified value (case sensitive).
          return vValue === undefined
            ? (v: TestVariantHistoryEntry) => negate !== (v.variant?.def?.[vKey] !== undefined)
            : (v: TestVariantHistoryEntry) => negate !== (v.variant?.def?.[vKey] === vValue);
        }
        default: {
          throw new Error(`invalid query type: ${queryType}`);
        }
      }
    });
  return (v) => filters.every((f) => f(v));
}

const VARIANT_FILTER_RE = /(-?)V:([^=]*)(?:=(.*))?$/i;

export type VariantFilter = (v: Variant) => boolean;

/**
 * Parses the variant predicate and variant filter from the query string. They
 * are useful for constructing RPC requests and filtering variant rows/groups.
 *
 * The returned VariantPredicate and VariantFilter may result in false
 * positives, so it should be used in conjunction of the TestVariantFilter
 * parsed from the same query.
 */
export function parseVariantFilter(filterQuery: string): [VariantPredicate, VariantFilter] {
  const predicate: DeepMutable<VariantPredicate> = { contains: { def: {} } };
  const filters: VariantFilter[] = [];
  for (const subQuery of filterQuery.split(' ')) {
    const match = subQuery.match(VARIANT_FILTER_RE);
    if (!match) {
      continue;
    }

    const [, neg, vKey, vValue] = match;
    const negate = neg === '-';
    if (vValue) {
      filters.push((v) => negate !== (v.def[vKey] === vValue));
    } else {
      filters.push((v) => negate !== (v.def[vKey] !== undefined));
    }

    if (!negate && vValue) {
      predicate.contains.def[vKey] = vValue;
    }
  }

  return [predicate, (v) => filters.every((f) => f(v))];
}

// Queries with predefined value.
const QUERY_SUGGESTIONS = [
  { value: 'Status:UNEXPECTED', explanation: 'Include only tests with unexpected status' },
  { value: '-Status:UNEXPECTED', explanation: 'Exclude tests with unexpected status' },
  { value: 'Status:UNEXPECTEDLY_SKIPPED', explanation: 'Include only tests with unexpectedly skipped status' },
  { value: '-Status:UNEXPECTEDLY_SKIPPED', explanation: 'Exclude tests with unexpectedly skipped status' },
  { value: 'Status:FLAKY', explanation: 'Include only tests with flaky status' },
  { value: '-Status:FLAKY', explanation: 'Exclude tests with flaky status' },
  { value: 'Status:EXONERATED', explanation: 'Include only tests with exonerated status' },
  { value: '-Status:EXONERATED', explanation: 'Exclude tests with exonerated status' },
  { value: 'Status:EXPECTED', explanation: 'Include only tests with expected status' },
  { value: '-Status:EXPECTED', explanation: 'Exclude tests with expected status' },
];

// Queries with arbitrary value.
const QUERY_TYPE_SUGGESTIONS = [
  { type: 'V:', explanation: 'Include only tests with a matching variant key-value pair (case sensitive)' },
  { type: '-V:', explanation: 'Exclude tests with a matching variant key-value pair (case sensitive)' },
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
        value: '-Status:EXPECTED',
        explanation: "Use '-' prefix to negate the filter",
      },
      {
        value: 'Status:UNEXPECTED -V:test_suite=browser_test',
        explanation: 'Use space to separate filters. Filters are logically joined with AND',
      },

      // Put this section behind `Advanced Syntax` so `Advanced Syntax` won't
      // be hidden after the size of supported filter types grows.
      {
        isHeader: true,
        display: html`<strong>Supported Filter Types</strong>`,
      },
      {
        value: 'V:variant-key=variant-value',
        explanation: 'Include only tests with a matching test variant key-value pair (case sensitive)',
      },
      {
        value: 'V:variant-key',
        explanation: 'Include only tests with the specified variant key (case sensitive)',
      },
      {
        value: 'Status:UNEXPECTED,UNEXPECTEDLY_SKIPPED,FLAKY,EXONERATED,EXPECTED',
        explanation: 'Include only tests with the specified status',
      },
    ];
  }

  const subQuery = query.split(' ').pop()!;
  if (subQuery === '') {
    return [];
  }

  const suggestions: Suggestion[] = [];

  // Suggest queries with predefined value.
  const subQueryUpper = subQuery.toUpperCase();
  suggestions.push(...QUERY_SUGGESTIONS.filter(({ value }) => value.toUpperCase().includes(subQueryUpper)));

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
