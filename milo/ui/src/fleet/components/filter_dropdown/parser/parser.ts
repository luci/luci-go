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

import { BLANK_VALUE } from '@/fleet/constants/filters';
import { SelectedOptions } from '@/fleet/types';

export type GetFiltersResult =
  | { filters: SelectedOptions; error: undefined }
  | { filters: undefined; error: Error };

/** Finds the matching ')'
 *  This function assumes the arg starts with a '('.
 * Note: This allows any weird characters inside the quotes
 */
const findClosingParen = (str: string): number => {
  if (!str || str[0] !== '(') {
    return -1;
  }

  const inQuote = false;
  let balance = 1;
  for (let i = 1; i < str.length; i++) {
    const char = str[i];

    if (char === '"' && str[i - 1] !== '\\') {
    } else if (char === '(' && !inQuote) {
      balance++;
    } else if (char === ')' && !inQuote) {
      balance--;
      if (balance === 0) {
        return i;
      }
    }
  }

  return -1;
};

/** Finds the matching quote, skipping all the escaped ones.
 *  This function assumes the arg starts with a quote.
 */
const findMatchingQuote = (str: string): number => {
  if (!str || str[0] !== '"') {
    return -1;
  }

  for (let i = 1; i < str.length; i++) {
    if (str[i] === '"' && str[i - 1] !== '\\') {
      return i;
    }
  }

  return -1;
};

/** Returns all the operands of expr key = ("value1" OR "value2" ...) */
const splitOrOperands = (str: string): string[] => {
  const result: string[] = [];

  let lastSplit = 0;
  let inQuotes = false;
  for (let i = 0; i < str.length; i++) {
    const char = str[i];

    if (char === '"' && str[i - 1] !== '\\') {
      inQuotes = !inQuotes;
    }

    if (!inQuotes) {
      // check if the substring starts with OR, ignoring whitespaces
      const match = str.substring(i).match(/^\s+OR\s+/);
      if (match) {
        result.push(str.substring(lastSplit, i));
        const delimiterLength = match[0].length;
        lastSplit = i + delimiterLength;
        i += delimiterLength - 1;
      }
    }
  }
  result.push(str.substring(lastSplit));

  return result.map((v) => v.trim().replace(/^"/, '').replace(/"$/, ''));
};

/**
 * Parses rhs part of the key1 = ("value1" OR "value2") or key = "value"
 */
const parseRhs = (
  str: string,
):
  | { values: string[]; rhsEndIdx: number; error: undefined }
  | { values: undefined; rhsEndIdx: -1; error: Error } => {
  let values: string[] = [];
  let rhsEndIdx = -1;

  const trimmedStr = str.trim();
  if (trimmedStr[0] === '(') {
    rhsEndIdx = findClosingParen(trimmedStr);
    if (rhsEndIdx === -1) {
      return {
        values: undefined,
        rhsEndIdx: -1,
        error: Error('Missing closing parenthesis'),
      };
    }

    values = splitOrOperands(trimmedStr.substring(1, rhsEndIdx));

    if (values.some((v) => v === '')) {
      return {
        values: undefined,
        rhsEndIdx: -1,
        error: Error('Found a hanging ORs'),
      };
    }
  } else if (trimmedStr[0] === '"') {
    rhsEndIdx = findMatchingQuote(trimmedStr);
    values = [trimmedStr.substring(1, rhsEndIdx)];
  } else {
    return {
      values: undefined,
      rhsEndIdx: -1,
      error: Error(
        `Unexpected character '${trimmedStr[0]}': should be one of '("'`,
      ),
    };
  }

  return { values, rhsEndIdx, error: undefined };
};

/** The input is expected to follow AIP - 160.
 * For now it's limited to inputs following the format:
 *   'key1 = ("value1" OR "value2") key = "value" NOT key3 (NOT key4 OR key4 =...)'.
 * Nested parentheses are not supported.
 * E.g.: 'fleet_labels.pool = ("default" OR "test")'
 * TODO: Consider moving this to a shared location
 */
export const parseFilters = (
  str: string,
  filters: SelectedOptions = {},
): GetFiltersResult => {
  let trimmedStr = str.trim();
  if (trimmedStr === '') {
    return { filters, error: undefined };
  }

  // Case 1: NOT key ...
  if (trimmedStr.startsWith('NOT')) {
    const match = trimmedStr.match(/^NOT\s+(\S+)\s?(.*)?$/);

    if (!match) {
      return {
        filters: undefined,
        error: Error('Invalid NOT expression, expecgted `NOT key ...`'),
      };
    }

    // TODO: Hotfix for b/449956551, needs further investigation on quote handling
    const key = match[1].replace(/labels\.([^"]+)/, 'labels."$1"');
    const rest = match[2] || '';

    return parseFilters(rest, {
      ...filters,
      [key]: [...(filters[key] ?? []), BLANK_VALUE],
    });
  }
  // Case 2: (NOT key OR ...)
  else if (trimmedStr.startsWith('(')) {
    // split the stuff inside parenthesis and the rest
    const closingParenIndex = findClosingParen(trimmedStr);
    if (closingParenIndex === -1) {
      return {
        filters: undefined,
        error: Error('Missing closing parenthesis'),
      };
    }

    const expr = trimmedStr.substring(1, closingParenIndex);
    const rest = trimmedStr.substring(closingParenIndex + 1);

    const parsedExpr = parseFilters(expr, filters);
    if (parsedExpr.error) {
      return parsedExpr;
    }

    return parseFilters(rest, parsedExpr.filters);
  }

  // Case 3: OR key =...
  // Note: if there is no whitespace between OR and key
  // we will treat OR as part of the key
  else if (trimmedStr.startsWith('OR ')) {
    // This works as later we combine newly found values
    // for a particular key instead of overwriting them
    trimmedStr = trimmedStr.replace('OR', '');
  }

  // Case 4: key =...
  const firstEqIdx = trimmedStr.indexOf('=');
  if (firstEqIdx === -1) return { filters, error: undefined };

  // TODO: Hotfix for b/449956551, needs further investigation on quote handling
  const key = trimmedStr
    .substring(0, firstEqIdx)
    .trim()
    .replace(/labels\.([^"]+)/, 'labels."$1"');
  const rest = trimmedStr.substring(firstEqIdx + 1).trim();

  const { values, rhsEndIdx, error } = parseRhs(rest);

  if (error) {
    return {
      filters: undefined,
      error: error,
    };
  }

  return parseFilters(rest.substring(rhsEndIdx + 1), {
    ...filters,
    [key]: [...(filters[key] ?? []), ...values],
  });
};

/**
 * The output is expected to follow AIP - 160.
 * For now it's limited to outputs following the format:
 *   "key1 = (value1 OR value2) key = value".
 * E.g.: "fleet_labels.pool = (default OR test)"
 * It also encloses values in quotes, as values can contain whitespaces,
 * and AIP-160 treats them as a whole.
 * More information: see the STRING description:
 * https://google.aip.dev/assets/misc/ebnf-filtering.txt
 * TODO: Consider moving this to a shared location
 */
export const stringifyFilters = (filters: SelectedOptions): string =>
  Object.entries(filters)
    .filter(([_key, values]) => values && values[0])
    .map(([key, values]) => {
      const actualValues = values.filter((v) => v !== BLANK_VALUE);
      const hasBlankFilter = values.length > actualValues.length;

      let valueFilter = '';
      if (actualValues.length === 1) {
        valueFilter = `${key} = "${actualValues[0]}"`;
      } else if (actualValues.length > 1) {
        valueFilter = `${key} = (${actualValues
          .map((v) => `"${v}"`)
          .join(' OR ')})`;
      }

      if (!hasBlankFilter) {
        return valueFilter;
      }

      if (!valueFilter) {
        return `NOT ${key}`;
      }

      return `(NOT ${key} OR ${valueFilter})`;
    })
    .join(' ');
