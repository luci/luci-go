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

import React from 'react';

function isStringArray(x: unknown): x is string[] {
  return Array.isArray(x) && x.every((element) => typeof element === 'string');
}

/**
 * Inspired by vscode fuzzy finding it requires that all the
 * characters in the query are present in the target in the same
 * order and priorities consecutive matches
 */
function fuzzySubstring(query: string, target: string): [number, number[]] {
  // Convert strings to lowercase for case-insensitive matching
  target = target.toLowerCase();
  query = query.toLowerCase();

  let targetIndex = 0;
  let queryIndex = 0;
  let score = 0;
  let seqMatchCount = 0;

  const matchesIdx = [];

  // Iterate through both strings
  while (targetIndex < target.length && queryIndex < query.length) {
    const targetChar = target[targetIndex];
    const queryChar = query[queryIndex];

    if (targetChar === queryChar) {
      // Characters match, increase score and sequential match count
      score += 1 + seqMatchCount * 5; // Sequential match bonus
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
    return [-1, []];
  }

  return [score, matchesIdx];
}

interface FuzzySortedElement<ChildrenElementType, ParentElementType> {
  el: ChildrenElementType;
  parent?: ParentElementType;
  label: string;
  score: number;
  matches: number[];
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
   * @returns all the matching items as a result object sorted based on
   * @param scoringFunction
   */
  <ParentElementType, ChildrenElementType>(
    list: ParentElementType[],
    getNestedOptions: (
      el: ParentElementType,
    ) => ChildrenElementType[] = () => [],
    getLabel?: (el: ParentElementType | ChildrenElementType) => string,
  ): FuzzySortedElement<ParentElementType, ChildrenElementType>[] => {
    if (!isStringArray(list) && getLabel === undefined) {
      throw Error(
        'If the list is not of type strings[] you need to provide a getter function',
      );
    }
    getLabel ??= String;

    return list
      .flatMap((el) => [
        { el: el, parent: undefined, label: getLabel(el) },
        ...getNestedOptions(el).map((o) => ({
          el: o,
          parent: el,
          label: getLabel(o),
        })),
      ])
      .map((obj) => {
        const [score, matches] = scoringFunction(searchString, obj.label);
        return {
          ...obj,
          score,
          matches,
        } as FuzzySortedElement<ParentElementType, ChildrenElementType>;
      })
      .filter(({ score }) => score >= minScore)
      .sort(({ score: score1 }, { score: score2 }) => score2 - score1);
  };

export function hasAnyModifier(
  e:
    | React.KeyboardEvent<HTMLDivElement>
    | React.KeyboardEvent<HTMLUListElement>,
) {
  return e.ctrlKey || e.altKey || e.metaKey || e.shiftKey;
}

/**
 * Handles up/down key to move to next/prev siblings and space to click.
 * Also works with cltr+j/k for up/down respectively.
 */
export function keyboardUpDownHandler(e: React.KeyboardEvent) {
  const target = e.target as HTMLElement;
  const parent = target.parentElement;
  const siblings = Array.from(parent?.children || []);

  let nextSibling: HTMLElement | undefined;
  let prevSibling: HTMLElement | undefined;

  const currentIndex = siblings.indexOf(target);

  switch (e.key) {
    case e.ctrlKey && 'j':
    case 'ArrowDown':
      if (siblings.length === 0) return;
      nextSibling = siblings[(currentIndex + 1) % siblings.length] as
        | HTMLElement
        | undefined;
      nextSibling?.focus();
      e.preventDefault();
      e.stopPropagation();
      break;
    case e.ctrlKey && 'k':
    case 'ArrowUp':
      prevSibling = siblings[
        (currentIndex - 1 + siblings.length) % siblings.length
      ] as HTMLElement | undefined;
      prevSibling?.focus();
      e.preventDefault();
      e.stopPropagation();
      break;
    case ' ':
      target.click();
      e.preventDefault();
      e.stopPropagation();
      break;
  }
}
