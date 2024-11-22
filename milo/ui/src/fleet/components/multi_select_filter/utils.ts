// Copyright 2024 The LUCI Authors.
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

/**
 * Inspired by vscode fuzzy finding it requires that all the
 * characters in the query are present in the target in the same
 * order and priorities consecutive matches
 */
function fuzzySubstring(query: string, target: string): number {
  // Convert strings to lowercase for case-insensitive matching
  target = target.toLowerCase();
  query = query.toLowerCase();

  let targetIndex = 0;
  let queryIndex = 0;
  let score = 0;
  let seqMatchCount = 0;

  // Iterate through both strings
  while (targetIndex < target.length && queryIndex < query.length) {
    const targetChar = target[targetIndex];
    const queryChar = query[queryIndex];

    if (targetChar === queryChar) {
      // Characters match, increase score and sequential match count
      score += 1 + seqMatchCount * 5; // Sequential match bonus
      seqMatchCount++;
      queryIndex++;
    } else {
      // No match, reset sequential match count
      seqMatchCount = 0;
    }

    targetIndex++;
  }

  // If we didn't reach the end of the query, it's not a match
  if (queryIndex < query.length) {
    return -1;
  }

  return score;
}

/**
 * @param searchString the string to search in the list
 * * @param minScore the minimum score an elements needs to have to be included in
 * the output list
 * @param scoringFunction the function used to score the matches, it should take
 * the query string as it's first parameter and the target as it's second. Bigger
 * scores are placed first in the output list
 *
 * @returns a sorting function
 */
export const fuzzySort =
  (
    searchString: string,
    minScore: number = 0,
    scoringFunction = fuzzySubstring,
  ) =>
    /**
     * @param list the list to sort
     * @param get a function to get a string from a list item, only required if
     * the input list is not string
     *
     * @retuns all the item with at least the @param minScore sorted based on
     * @param scoringFunction
     */
    <T>(
      list: T[],
      get?: T extends string ? undefined : (x: T) => string,
    ): T[] => {
      if (
        list.some((element) => typeof element !== 'string') &&
        get === undefined
      ) {
        throw Error(
          'If the list is not of strings you need to provide a getter function',
        );
      }

      if (searchString.length === 0) {
        return list;
      }

      const out = list
        .map((s) => {
          const string = get ? get(s) : (s as string);
          return [s, scoringFunction(searchString, string)] as const;
        })
        .filter(([_, score]) => score >= minScore)
        .sort(([_, score1], [__, score2]) => score2 - score1)
        .map(([s, _]) => s);

      return out;
    };
