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

import { MILO_PROD } from './builds';

export function hasAnyModifier(e: React.KeyboardEvent<HTMLElement>) {
  return e.ctrlKey || e.altKey || e.metaKey || e.shiftKey;
}

/**
 * Handles keyboard navigation for list-like structures.
 *
 * This function is designed to be used as a keyboard event handler on a container
 * element that holds a list of items. It provides the following functionality:
 *
 * - **Arrow Keys & `Ctrl+j/k`**: Allows the user to move focus up and down the list
 *   using the arrow keys (`ArrowUp`, `ArrowDown`) or `Ctrl+j` (down) and `Ctrl+k` (up).
 *   The navigation wraps around, so pressing down on the last item moves focus to
 *   the first, and vice versa.
 *
 * - **Spacebar**: When an item is focused, pressing the spacebar will trigger a
 *   `click` event on that item. This is useful for selecting or activating items.
 *   If the focused element is an `<input>`, the spacebar will behave as usual
 *   (i.e., it will type a space).
 *
 * - **Flexible Structure**: The handler is designed to work with complex DOM
 *   structures. It finds the closest `<ul>` ancestor of the event target and
 *   then identifies all navigable items within that list. Navigable items are
 *   defined by the selector `'#search, [role="menuitem"], button'`. This allows
 *   for a mix of list items, search bars, and buttons to be included in the
- *   navigation flow.
 */
export function keyboardListNavigationHandler(
  e: React.KeyboardEvent,
  goToNext?: () => void,
  goToPrevious?: () => void,
  direction: 'vertical' | 'horizontal' = 'vertical',
) {
  const target = e.target as HTMLElement;

  // If the user is typing a space in an input, do not prevent it.
  if (e.key === ' ' && target.nodeName === 'INPUT') {
    return;
  }

  const handleGoToNext = () => {
    if (goToNext) {
      goToNext();
    } else {
      const neighbors = findNeighborsInList(e.target as HTMLElement);
      neighbors?.next.focus();
      e.preventDefault();
      e.stopPropagation();
    }
  };

  const handleGoToPrevious = () => {
    if (goToPrevious) {
      goToPrevious();
    } else {
      const neighbors = findNeighborsInList(e.target as HTMLElement);
      neighbors?.previous.focus();
      e.preventDefault();
      e.stopPropagation();
    }
  };

  switch (e.key) {
    case e.ctrlKey && 'h':
    case 'ArrowLeft':
      if (direction === 'horizontal') {
        handleGoToPrevious();
      }
      break;
    case e.ctrlKey && 'l':
    case 'ArrowRight':
      if (direction === 'horizontal') {
        handleGoToNext();
      }
      break;
    case e.ctrlKey && 'k':
    case 'ArrowUp':
      if (direction === 'vertical') {
        handleGoToPrevious();
      }
      break;
    case e.ctrlKey && 'j':
    case 'ArrowDown':
      if (direction === 'vertical') {
        handleGoToNext();
      }
      break;
    case ' ':
      target.click();
      e.preventDefault();
      e.stopPropagation();
      break;
  }
}

const findNeighborsInList = (target: HTMLElement) => {
  const listContainer = target.closest('ul');
  if (!listContainer) {
    return undefined;
  }

  const navigableItems = Array.from(
    listContainer.querySelectorAll<HTMLElement>(
      '#search, [role="menuitem"], button',
    ),
  );
  if (navigableItems.length === 0) {
    return undefined;
  }

  const currentIndex = navigableItems.findIndex((item) =>
    item.contains(target),
  );
  if (currentIndex === -1) {
    return undefined;
  }

  return {
    previous:
      navigableItems[
        (currentIndex - 1 + navigableItems.length) % navigableItems.length
      ],
    next: navigableItems[(currentIndex + 1) % navigableItems.length],
  };
};

export const isProdEnvironment = () => {
  return window.location.hostname === MILO_PROD;
};
