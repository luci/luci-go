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

function isStringArray(x: unknown): x is string[] {
  return Array.isArray(x) && x.every((element) => typeof element === 'string');
}

/**
 * @param list the list to sort
 * @param getLabel a function to get a string from a list item, only required if
 * the input list is not string
 *
 * @returns all the matching items as a result object sorted based on
 * @param scoringFunction
 */
export type SortingFunction = <ElementType>(
  list: ElementType[],
  getLabel?: (el: ElementType) => string,
) => SortedElement<ElementType>[];

export type FuzzySort = (
  searchString: string,
  scoringFunction?: ScoringFunction,
) => SortingFunction;

export type ScoringFunction = (
  query: string,
  target: string,
) => [number, number[]];

export type SortedElement<ElementType> = {
  el: ElementType;
  score: number;
  matches: number[];
};

/** The number of characters from the start of the string to apply a bonus for. */
const START_OF_STRING_BONUS_CHAR_LIMIT = 5;
/** The number of characters from the end of the string to apply a bonus for. */
const END_OF_STRING_BONUS_CHAR_LIMIT = 5;

/**
 * Calculates a bonus score for a character match based on its proximity to the
 * start or end of a string. The bonus is on a quadratic scale, from 1 (at the
 * edge) to 0 (at or beyond the character limit).
 */
const calculateEdgeProximityBonus = (
  distanceFromEdge: number,
  bonusCharLimit: number,
) => {
  return (
    ((bonusCharLimit - Math.min(distanceFromEdge, bonusCharLimit)) /
      bonusCharLimit) **
    2
  );
};

/**
 * Inspired by vscode fuzzy finding it requires that all the
 * characters in the query are present in the target in the same
 * order and priorities consecutive matches
 */
export const fuzzySubstring: ScoringFunction = (
  query: string,
  target: string,
) => {
  // Convert strings to lowercase for case-insensitive matching
  target = target.toLowerCase();
  query = query.toLowerCase();

  let targetIndex = 0;
  let queryIndex = 0;
  let score = 0.0;
  let seqMatchCount = 0;

  const matchesIdx = [];

  // Iterate through both strings
  while (targetIndex < target.length && queryIndex < query.length) {
    const targetChar = target[targetIndex];
    const queryChar = query[queryIndex];

    if (targetChar === queryChar) {
      // Characters match, increase score and sequential match count
      score += 1.0 + seqMatchCount * 5.0; // Sequential match bonus

      const startCharactersBonus = calculateEdgeProximityBonus(
        targetIndex,
        START_OF_STRING_BONUS_CHAR_LIMIT,
      );
      const endCharactersBonus = calculateEdgeProximityBonus(
        target.length - 1 - targetIndex,
        END_OF_STRING_BONUS_CHAR_LIMIT,
      );

      score += startCharactersBonus + endCharactersBonus * 0.9;

      seqMatchCount++;
      queryIndex++;

      matchesIdx.push(targetIndex);
    } else {
      // No match, reset sequential match count
      seqMatchCount = 0;
    }

    targetIndex++;
  }

  // If we didn't reach the end of the query, it's not a match
  if (queryIndex < query.length) {
    return [-1.0, []];
  }

  const exactMatchBonus = matchesIdx.length === target.length ? 10.0 : 0.0;
  score += exactMatchBonus;

  return [score, matchesIdx];
};

/**
 * @param searchString the string to search in the list
 * @param scoringFunction the function used to score the matches, it should take
 * the query string as it's first parameter and the target as it's second. Bigger
 * scores are placed first in the output list
 *
 * @returns a sorting function
 */
export const fuzzySort: FuzzySort =
  (searchString, scoringFunction = fuzzySubstring) =>
  (list, getLabel) => {
    if (!isStringArray(list) && getLabel === undefined) {
      throw Error(
        'If the list is not of type strings[] you need to provide a getter function',
      );
    }
    getLabel ??= String;

    return list
      .map((obj) => {
        const [score, matches] = scoringFunction(searchString, getLabel(obj));
        return {
          el: obj,
          score,
          matches,
        };
      })
      .sort(({ score: score1 }, { score: score2 }) => score2 - score1);
  };
