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
  {value: 'Status:UNEXPECTED', explanation: 'Include only tests that are unexpected'},
  {value: '-Status:UNEXPECTED', explanation: 'Exclude tests that are unexpected'},
  {value: 'Status:FLAKY', explanation: 'Include only tests that are flaky'},
  {value: '-Status:FLAKY', explanation: 'Exclude tests that are flaky'},
  {value: 'Status:EXONERATED', explanation: 'Include only tests that are exonerated'},
  {value: '-Status:EXONERATED', explanation: 'Exclude tests that are exonerated'},
  {value: 'Status:EXPECTED', explanation: 'Include only tests that are expected'},
  {value: '-Status:EXPECTED', explanation: 'Exclude tests that are expected'},

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

function getIdQuerySuggestion(substr: string, neg: boolean): Suggestion {
  return {
    value: `${neg ? '-' : ''}ID:${substr}`,
    explanation: `${neg ? 'Exclude' : 'Include only'} tests with the specified substring in their ID (case insensitive)`,
  };
}

export function suggestSearchQuery(subQuery: string): readonly Suggestion[] {
  if (subQuery === '') {
    return [];
  }

  const match = subQuery.match(/^(?<neg>-?)ID:(?<substr>.*)/);
  if (match) {
    if (match.groups!['neg'] === '-') {
      return [getIdQuerySuggestion(match.groups!['substr'], true)];
    }
    return [
      getIdQuerySuggestion(match.groups!['substr'], false),
      getIdQuerySuggestion(match.groups!['substr'], true),
    ];
  }

  const subQueryUpper = subQuery.toUpperCase();
  const suggestions = QUERY_SUGGESTIONS.filter(({value}) => value.toUpperCase().includes(subQueryUpper));
  suggestions.push(
    getIdQuerySuggestion(subQuery, false),
    getIdQuerySuggestion(subQuery, true),
  );
  return suggestions;
}
