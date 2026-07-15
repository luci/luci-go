// Copyright 2026 The LUCI Authors.
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
 * Determines if a ClickAwayListener event should be ignored.
 *
 * Specifically handles:
 * 1. Target nodes that were synchronously unmounted from the DOM upon click (!target.isConnected).
 * 2. Target nodes that are contained within the dropdown's anchor element.
 */
export function shouldIgnoreClickAway(
  event: MouseEvent | TouchEvent,
  anchorEl: HTMLElement | Element | null | undefined,
  containerEl?: HTMLElement | Element | null,
): boolean {
  const path =
    'composedPath' in event && typeof event.composedPath === 'function'
      ? event.composedPath()
      : [];
  const target = (path.length > 0 ? path[0] : event.target) as Node | null;

  if (!target) {
    return false;
  }

  // 1. Ignore events originating inside the anchor element
  if (
    anchorEl &&
    'contains' in anchorEl &&
    typeof anchorEl.contains === 'function'
  ) {
    if (anchorEl.contains(target) || path.includes(anchorEl)) {
      return true;
    }
  }

  // 2. If containerEl is provided, check if the click originated inside our container
  if (
    containerEl &&
    'contains' in containerEl &&
    typeof containerEl.contains === 'function'
  ) {
    if (containerEl.contains(target) || path.includes(containerEl)) {
      return true;
    }
  }

  // 3. Ignore events from detached nodes (elements unmounted upon click) only if
  // they originated inside our dropdown hierarchy (Popper, Paper, Popover, menu, or anchor/container).
  if (!target.isConnected) {
    if (
      path.some(
        (node) =>
          node === anchorEl ||
          node === containerEl ||
          (node instanceof Element &&
            node.matches(
              '.MuiPopper-root, .MuiPaper-root, .MuiPopover-root, [role="menu"], [role="listbox"], [role="presentation"]',
            )),
      )
    ) {
      return true;
    }
  }

  return false;
}
