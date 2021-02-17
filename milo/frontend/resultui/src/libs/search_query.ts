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

import { html } from 'lit-html';
import { Suggestion } from '../components/auto_complete';
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
      // Whether the test variant has the specified status.
      case 'STATUS': {
        const statuses = value.split(',');
        return (v: TestVariant) => negate !== statuses.includes(v.status);
      }
      // Whether there's at least one a test result of the specified status.
      case 'RSTATUS': {
        const statuses = value.split(',');
        return (v: TestVariant) => negate !== (v.results || []).some((r) => statuses.includes(r.result.status));
      // Whether the test ID contains the query as a substring (case insensitive).
      }
      case 'ID': {
        return (v: TestVariant) => negate !== v.testId.toUpperCase().includes(value);
      }
      default: {
        throw new Error(`invalid query type: ${type}`);
      }
    }
  });
  return (v) => filters.every((f) => f(v));
}

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

function getIdQuerySuggestion(substr: string, neg: boolean, bold: boolean): Suggestion {
  return {
    value: `${neg ? '-' : ''}ID:${substr}`,
    display: bold ? html`<strong>${neg ? '-' : ''}ID:</strong>${substr}` : undefined,
    explanation: `${neg ? 'Exclude' : 'Include only'} tests with the specified substring in their ID (case insensitive)`,
  };
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
      {value: 'ID:test-id-substr', explanation: 'Include only tests with the specified substring in their ID (case insensitive). `ID:` is optional'},
      {value: 'Status:UNEXPECTED,UNEXPECTEDLY_SKIPPED,FLAKY,EXONERATED,EXPECTED', explanation: 'Include only tests with the specified status'},
      {value: 'RStatus:Pass,Fail,Crash,Abort,Skip', explanation: 'Include only tests with at least one run of the specified status'},
    ];
  }

  const subQuery = query.split(' ').pop()!;
  if (subQuery === '') {
    return [];
  }

  const match = subQuery.match(/^(?<neg>-?)ID:(?<substr>.*)/);
  if (match) {
    if (match.groups!['neg'] === '-') {
      return [getIdQuerySuggestion(match.groups!['substr'], true, true)];
    }
    return [
      getIdQuerySuggestion(match.groups!['substr'], false, true),
      getIdQuerySuggestion(match.groups!['substr'], true, true),
    ];
  }

  const subQueryUpper = subQuery.toUpperCase();
  const suggestions = QUERY_SUGGESTIONS
    .map<Suggestion>(({value, explanation}) => {
      const matchIndex = value.toUpperCase().search(subQueryUpper);
      if (matchIndex === -1) {
        return {value, explanation};
      }
      // Highlight the matched portion.
      const prefix = value.slice(0, matchIndex);
      const matched = value.slice(matchIndex, matchIndex + subQuery.length);
      const suffix = value.slice(matchIndex + subQuery.length);
      return {value, explanation, display: html`${prefix}<strong>${matched}</strong>${suffix}`};
    })
    .filter(({display}) => display);
  suggestions.push(
    getIdQuerySuggestion(subQuery, false, false),
    getIdQuerySuggestion(subQuery, true, false),
  );
  return suggestions;
}
