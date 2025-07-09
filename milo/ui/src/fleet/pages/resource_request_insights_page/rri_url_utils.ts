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

// This file contains helper methods similar to what search_param_utils does for devices list.
// The difference currently is that RRI uses custom format for storing filters in the URL
// supporting different kinds of filters, while devices list tries to follow AIP-160
// This should probably replace / be merged with original devices list logic as it provides an expansion of existing functionality
// TODO: b/422126877
export const multiselectFilterToUrlString = (values: string[]): string => {
  if (values.length === 0) {
    return '';
  }

  if (values.length === 1) {
    return '"' + escapeQuotes(values[0]) + '"';
  }

  return (
    '(' + values.map((v) => '"' + escapeQuotes(v) + '"').join(' OR ') + ')'
  );
};

export const parseMultiselectFilter = (str?: string): string[] | undefined => {
  if (!str) {
    return undefined;
  }

  const trimmed = str.trim();
  if (trimmed.startsWith('"') && trimmed.endsWith('"')) {
    // single value in quotes
    return [trimmed.slice(1, -1)];
  }
  if (trimmed.startsWith('(')) {
    // multiple values in parentheses
    if (!trimmed.endsWith(')')) {
      throw new Error('Missing closing parenthesis');
    }

    const values = trimmed
      .slice(1, -1)
      .split(/OR/)
      .map((s) => s.trim());

    if (values.some((v) => v === '')) {
      throw new Error('Found a hanging ORs');
    }

    return values.map((v) => {
      const quotesCount = (v.match(/"/g) || []).length;
      const escapedQuotesCount = (v.match(/\\"/g) || []).length;
      const unescapedQuotesCount = quotesCount - escapedQuotesCount;
      if (unescapedQuotesCount !== 0 && unescapedQuotesCount !== 2) {
        throw new Error('Wrong number of quotes in value: ' + v);
      }

      if (unescapedQuotesCount === 2) {
        if (!v.startsWith('"') || !v.endsWith('"')) {
          throw new Error('Missing closing quote');
        }
        return processEscapedQuotes(v.slice(1, -1));
      }

      return processEscapedQuotes(v);
    });
  }

  // single value without quotes, we should return untrimmed one
  return [str];
};

const processEscapedQuotes = (str: string): string => {
  return str.replace(/\\"/g, '"');
};

const escapeQuotes = (str: string): string => {
  return str.replace(/"/g, '\\"');
};
