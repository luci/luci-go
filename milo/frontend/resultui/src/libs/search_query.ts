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

import { html, TemplateResult } from 'lit-html';
import { Suggestion } from '../components/auto_complete';
import { TestVariant } from '../services/resultdb';

const SPECIAL_QUERY_RE = /^(-?)([a-zA-Z]+):(.+)$/;

export type TestVariantFilter = (v: TestVariant) => boolean;

export function parseSearchQuery(searchQuery: string): TestVariantFilter {
  const filters = searchQuery.toUpperCase().split(' ').map((query) => {
    const match = query.match(SPECIAL_QUERY_RE);

    // If the query isn't a special query, treat it as an ID query.
    const [, neg, type, value] = match || ['', '', 'ID', query];
    const negate = neg === '-';
    switch (type) {
      // Whether the test variant has the specified status.
      case 'STATUS': {
        const statuses = value.split(',');
        return (v: TestVariant) => negate !== statuses.includes(v.status);
      }
      // Whether there's at least one a test result of the specified status.
      case 'RSTATUS': {
        const statuses = value.split(',');
        return (v: TestVariant) => negate !== (v.results || []).some((r) => statuses.includes(r.result.status));
      }
      // Whether the test ID contains the query as a substring (case insensitive).
      case 'ID': {
        return (v: TestVariant) => negate !== v.testId.toUpperCase().includes(value);
      }
      // Whether the test variant has the specified variant hash.
      case 'VHASH': {
        return (v: TestVariant) => negate !== (v.variantHash.toUpperCase() === value);
      }
      default: {
        throw new Error(`invalid query type: ${type}`);
      }
    }
  });
  return (v) => filters.every((f) => f(v));
}

// Queries with predefined value.
const QUERY_SUGGESTIONS = [
  {value: 'Status:UNEXPECTED', explanation: 'Include only tests with unexpected status'},
  {value: '-Status:UNEXPECTED', explanation: 'Exclude tests with unexpected status'},
  {value: 'Status:UNEXPECTEDLY_SKIPPED', explanation: 'Include only tests with unexpectedly skipped status'},
  {value: '-Status:UNEXPECTEDLY_SKIPPED', explanation: 'Exclude tests with unexpectedly skipped status'},
  {value: 'Status:FLAKY', explanation: 'Include only tests with flaky status'},
  {value: '-Status:FLAKY', explanation: 'Exclude tests with flaky status'},
  {value: 'Status:EXONERATED', explanation: 'Include only tests with exonerated status'},
  {value: '-Status:EXONERATED', explanation: 'Exclude tests with exonerated status'},
  {value: 'Status:EXPECTED', explanation: 'Include only tests with expected status'},
  {value: '-Status:EXPECTED', explanation: 'Exclude tests with expected status'},

  {value: 'RStatus:Pass', explanation: 'Include only tests with at least one passed run'},
  {value: '-RStatus:Pass', explanation: 'Exclude tests with at least one passed run'},
  {value: 'RStatus:Fail', explanation: 'Include only tests with at least one failed run'},
  {value: '-RStatus:Fail', explanation: 'Exclude tests with at least one failed run'},
  {value: 'RStatus:Crash', explanation: 'Include only tests with at least one crashed run'},
  {value: '-RStatus:Crash', explanation: 'Exclude tests with at least one crashed run'},
  {value: 'RStatus:Abort', explanation: 'Include only tests with at least one aborted run'},
  {value: '-RStatus:Abort', explanation: 'Exclude tests with at least one aborted run'},
  {value: 'RStatus:Skip', explanation: 'Include only tests with at least one skipped run'},
  {value: '-RStatus:Skip', explanation: 'Exclude tests with at least one skipped run'},
];

// Queries with arbitrary value.
const QUERY_TYPE_SUGGESTIONS = [
  {type: 'ID:', explanation: 'Include only tests with the specified substring in their ID (case insensitive)'},
  {type: '-ID:', explanation: 'Exclude tests with the specified substring in their ID (case insensitive)'},

  {type: 'VHash:', explanation: 'Include only tests with the specified variant hash'},
  {type: '-VHash:', explanation: 'Exclude tests with the specified variant hash'},
];

/**
 * Return a lit-html template that highlight the substring (case-insensitive)
 * in the given fullString.
 */
function highlight(fullString: string, subString: string): TemplateResult {
  const matchStart = fullString.toUpperCase().search(subString.toUpperCase());
  const prefix = fullString.slice(0, matchStart);
  const matched = fullString.slice(matchStart, matchStart + subString.length);
  const suffix = fullString.slice(matchStart + subString.length);
  return html`${prefix}<strong>${matched}</strong>${suffix}`;
}

export function suggestSearchQuery(query: string): readonly Suggestion[] {
  if (query === '') {
    // Return some example queries when the query is empty.
    return [
      {isHeader: true, display: html`<strong>Advanced Syntax</strong>`},
      {value: '-Status:EXPECTED', explanation: 'Use \'-\' prefix to negate the filter'},
      {value: 'Status:UNEXPECTED -RStatus:Skipped', explanation: 'Use space to separate filters. Filters are logically joined with AND'},

      // Put this section behind `Advanced Syntax` so `Advanced Syntax` won't
      // be hidden after the size of supported filter types grows.
      {isHeader: true, display: html`<strong>Supported Filter Types</strong>`},
      {value: 'ID:test-id-substr', explanation: 'Include only tests with the specified substring in their ID (case insensitive)'},
      {value: 'test-id-substr', explanation: 'Filters with no type are treated as ID filters'},
      {value: 'Status:UNEXPECTED,UNEXPECTEDLY_SKIPPED,FLAKY,EXONERATED,EXPECTED', explanation: 'Include only tests with the specified status'},
      {value: 'RStatus:Pass,Fail,Crash,Abort,Skip', explanation: 'Include only tests with at least one run of the specified status'},
      {value: 'VHash:2660cde9da304c42', explanation: 'Include only tests with the specified variant hash'},
    ];
  }

  const subQuery = query.split(' ').pop()!;
  if (subQuery === '') {
    return [];
  }

  const suggestions: Suggestion[] = [];

  // Suggest queries with predefined value.
  const subQueryUpper = subQuery.toUpperCase();
  suggestions.push(...QUERY_SUGGESTIONS.filter(({value}) => value.toUpperCase().includes(subQueryUpper)));

  // Suggest queries with arbitrary value.
  const match = subQuery.match(/^([^:]*:?)(.*)$/);
  if (match) {
    const [, subQueryType, subQueryValue] = match;
    const typeUpper = subQueryType.toUpperCase();
    suggestions.push(
      ...QUERY_TYPE_SUGGESTIONS.flatMap(({type, explanation}) => {
        if (type.toUpperCase().includes(typeUpper)) {
          return [{value: type + subQueryValue, explanation}];
        }

        if (subQueryValue === '') {
          return [{value: type + subQueryType, explanation}];
        }

        return [];
      }),
    );
  }

  return suggestions.map((s) => ({...s, display: s.display || highlight(s.value!, subQuery)}));
}
