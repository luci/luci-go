// Copyright 2025 The LUCI Authors.
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

import { useCallback, useMemo } from 'react';

import {
  SearchMeasurementsFilter,
  SearchMeasurementsRequest,
} from '@/crystal_ball/hooks/use_android_perf_api';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

/**
 * URL Search Parameter that corresponds to the serialized
 * SearchMeasurementsRequest.
 */
export const URL_SEARCH_QUERY_PARAM = 'q';

/**
 * Special characters used by the serialized query.
 */
enum QueryParts {
  DELIMITER = ',',
  EQUALITY = ':',
  SEPARATOR = ' ',
  QUOTE = '"',
  ESCAPE = '\\',
}

/**
 * Regex to test if a string value needs to be quoted.
 */
const NEEDS_QUOTING_REGEX = new RegExp(
  `[${QueryParts.SEPARATOR}${QueryParts.DELIMITER}${QueryParts.EQUALITY}${QueryParts.QUOTE}]`,
);

/**
 * Quotes a string value if necessary.
 */
const quoteValue = (value: string): string => {
  if (!NEEDS_QUOTING_REGEX.test(value)) {
    return value;
  }
  const escapedValue = value.replace(/\\/g, '\\\\').replace(/"/g, '\\"');
  return `${QueryParts.QUOTE}${escapedValue}${QueryParts.QUOTE}`;
};

/**
 * Unquotes a string value if it's quoted.
 */
const unquoteValue = (value: string): string => {
  if (value.startsWith(QueryParts.QUOTE) && value.endsWith(QueryParts.QUOTE)) {
    try {
      return value
        .substring(1, value.length - 1)
        .replace(/\\"/g, '"')
        .replace(/\\\\/g, '\\');
    } catch {
      // Return original if error
      return value;
    }
  }
  return value;
};

/**
 * Define a fixed order for serialization.
 */
const SERIALIZATION_ORDER: SearchMeasurementsFilter[] = [
  SearchMeasurementsFilter.TEST,
  SearchMeasurementsFilter.ATP_TEST,
  SearchMeasurementsFilter.BUILD_BRANCH,
  SearchMeasurementsFilter.BUILD_TARGET,
  SearchMeasurementsFilter.LAST_N_DAYS,
  SearchMeasurementsFilter.BUILD_CREATE_START_TIME,
  SearchMeasurementsFilter.BUILD_CREATE_END_TIME,
  SearchMeasurementsFilter.METRIC_KEYS,
  SearchMeasurementsFilter.EXTRA_COLUMNS,
];

/**
 * Helper function to serialize SearchMeasurementsRequest to a string.
 * @param request - a partial SearchMeasurementsRequest.
 * @returns A string representation for the URL.
 */
const serializeSearchRequest = (
  request: Partial<SearchMeasurementsRequest>,
): string => {
  const params: string[] = [];
  const eq = QueryParts.EQUALITY;

  SERIALIZATION_ORDER.forEach((filter) => {
    switch (filter) {
      case SearchMeasurementsFilter.TEST:
        if (request.testNameFilter)
          params.push(`${filter}${eq}${quoteValue(request.testNameFilter)}`);
        break;
      case SearchMeasurementsFilter.ATP_TEST:
        if (request.atpTestNameFilter)
          params.push(`${filter}${eq}${quoteValue(request.atpTestNameFilter)}`);
        break;
      case SearchMeasurementsFilter.BUILD_BRANCH:
        if (request.buildBranch)
          params.push(`${filter}${eq}${quoteValue(request.buildBranch)}`);
        break;
      case SearchMeasurementsFilter.BUILD_TARGET:
        if (request.buildTarget)
          params.push(`${filter}${eq}${quoteValue(request.buildTarget)}`);
        break;
      case SearchMeasurementsFilter.LAST_N_DAYS:
        if (request.lastNDays !== undefined)
          params.push(`${filter}${eq}${request.lastNDays}`);
        break;
      case SearchMeasurementsFilter.BUILD_CREATE_START_TIME:
        if (request.buildCreateStartTime?.seconds)
          params.push(`${filter}${eq}${request.buildCreateStartTime.seconds}`);
        break;
      case SearchMeasurementsFilter.BUILD_CREATE_END_TIME:
        if (request.buildCreateEndTime?.seconds)
          params.push(`${filter}${eq}${request.buildCreateEndTime.seconds}`);
        break;
      case SearchMeasurementsFilter.METRIC_KEYS:
        if (request.metricKeys?.length)
          params.push(
            `${filter}${eq}${request.metricKeys
              .map(quoteValue)
              .join(QueryParts.DELIMITER)}`,
          );
        break;
      case SearchMeasurementsFilter.EXTRA_COLUMNS:
        if (request.extraColumns?.length)
          params.push(
            `${filter}${eq}${request.extraColumns
              .map(quoteValue)
              .join(QueryParts.DELIMITER)}`,
          );
        break;
    }
  });
  return params.join(QueryParts.SEPARATOR);
};

/**
 * Regex to split comma-separated values, respecting quotes.
 */
const COMMA_SEPARATED_REGEX = /(?:"(?:[^"\\]|\\.)*")|[^,]+/g;

/**
 * Helper function to deserialize a string to SearchMeasurementsRequest.
 */
const deserializeSearchRequest = (
  query: string,
): Partial<SearchMeasurementsRequest> => {
  const request: Partial<SearchMeasurementsRequest> = {};
  const orderedKeys = Object.values(SearchMeasurementsFilter);
  const keyPattern = orderedKeys.join('|');
  const eq = QueryParts.EQUALITY;

  const keyMatchRegex = new RegExp(`(${keyPattern})${eq}`, 'g');
  const matches = Array.from(query.matchAll(keyMatchRegex));

  for (let i = 0; i < matches.length; i++) {
    const match = matches[i];
    const key = match[1] as SearchMeasurementsFilter;
    const startIndex = match.index! + match[0].length;
    const nextMatch = matches[i + 1];
    let endIndex = query.length;
    if (nextMatch) {
      // Find the space separator before the next key
      const potentialEndIndex = query.lastIndexOf(
        QueryParts.SEPARATOR,
        nextMatch.index! - 1,
      );
      if (potentialEndIndex > startIndex) {
        endIndex = potentialEndIndex;
      }
    }

    const value = query.substring(startIndex, endIndex).trim();

    if (value) {
      try {
        switch (key) {
          case SearchMeasurementsFilter.TEST:
            request.testNameFilter = unquoteValue(value);
            break;
          case SearchMeasurementsFilter.ATP_TEST:
            request.atpTestNameFilter = unquoteValue(value);
            break;
          case SearchMeasurementsFilter.BUILD_BRANCH:
            request.buildBranch = unquoteValue(value);
            break;
          case SearchMeasurementsFilter.BUILD_TARGET:
            request.buildTarget = unquoteValue(value);
            break;
          case SearchMeasurementsFilter.LAST_N_DAYS: {
            const num = parseInt(value, 10);
            if (!isNaN(num)) request.lastNDays = num;
            break;
          }
          case SearchMeasurementsFilter.BUILD_CREATE_START_TIME: {
            const num = parseInt(value, 10);
            if (!isNaN(num))
              request.buildCreateStartTime = { seconds: num, nanos: 0 };
            break;
          }
          case SearchMeasurementsFilter.BUILD_CREATE_END_TIME: {
            const num = parseInt(value, 10);
            if (!isNaN(num))
              request.buildCreateEndTime = { seconds: num, nanos: 0 };
            break;
          }
          case SearchMeasurementsFilter.METRIC_KEYS: {
            const metricValues = value.match(COMMA_SEPARATED_REGEX);
            if (metricValues) {
              request.metricKeys = metricValues
                .map(unquoteValue)
                .filter((s) => s);
            } else {
              request.metricKeys = [];
            }
            break;
          }
          case SearchMeasurementsFilter.EXTRA_COLUMNS: {
            const colValues = value.match(COMMA_SEPARATED_REGEX);
            if (colValues) {
              request.extraColumns = colValues
                .map(unquoteValue)
                .filter((s) => s);
            } else {
              request.extraColumns = [];
            }
            break;
          }
        }
      } catch {
        // Skip malformed query parts.
      }
    }
  }
  return request;
};

interface UseSearchQuerySyncReturn {
  searchRequestFromUrl: Partial<SearchMeasurementsRequest>;
  updateSearchQuery: (request: Partial<SearchMeasurementsRequest>) => void;
  clearSearchQuery: () => void;
}

export function useSearchQuerySync(): UseSearchQuerySyncReturn {
  const [searchParams, setSearchParams] = useSyncedSearchParams();

  const searchRequestFromUrl = useMemo(() => {
    const q = searchParams.get(URL_SEARCH_QUERY_PARAM);
    return q ? deserializeSearchRequest(q) : {};
  }, [searchParams]);

  const updateSearchQuery = useCallback(
    (request: Partial<SearchMeasurementsRequest>) => {
      const filteredRequest = Object.fromEntries(
        Object.entries(request).filter(([, value]) => value !== undefined),
      );
      const query = serializeSearchRequest(filteredRequest);
      if (query) {
        setSearchParams(
          (params) => {
            params.set(URL_SEARCH_QUERY_PARAM, query);
            return params;
          },
          { replace: true },
        );
      } else {
        setSearchParams(
          (params) => {
            params.delete(URL_SEARCH_QUERY_PARAM);
            return params;
          },
          { replace: true },
        );
      }
    },
    [setSearchParams],
  );

  const clearSearchQuery = useCallback(() => {
    setSearchParams(
      (params) => {
        params.delete(URL_SEARCH_QUERY_PARAM);
        return params;
      },
      { replace: true },
    );
  }, [setSearchParams]);

  return { searchRequestFromUrl, updateSearchQuery, clearSearchQuery };
}
