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

export function hasAnyModifier(e: React.KeyboardEvent<HTMLElement>) {
  return e.ctrlKey || e.altKey || e.metaKey || e.shiftKey;
}

/**
 * Handles up/down key to move to next/prev siblings and space to click.
 * Also works with cltr+j/k for up/down respectively.
 */
export function keyboardUpDownHandler(e: React.KeyboardEvent) {
  const target = e.target as HTMLElement;
  const parent = target.parentElement;
  const siblings = Array.from(parent?.children || []).filter(
    (el) => el.nodeName === 'LI' || el.id === 'search',
  );

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
