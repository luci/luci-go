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

import { RefObject, useEffect, useState } from 'react';

/**
 * Options for the useHasBeenVisible hook.
 */
export interface UseHasBeenVisibleOptions {
  /**
   * Whether visibility tracking is enabled. When false, the element is treated
   * as visible immediately.
   */
  enabled?: boolean;
  /**
   * Margin around the viewport used to detect visibility, in CSS margin
   * syntax. A positive margin reports elements as visible shortly before they
   * scroll into view.
   */
  rootMargin?: string;
}

/**
 * Returns whether the referenced element has ever intersected the viewport
 * (expanded by `rootMargin`). Once true, it stays true, so content that has
 * been revealed is not hidden again when it scrolls out of view.
 *
 * Falls back to `true` when `IntersectionObserver` is not available.
 * @param ref - Ref to the element to observe.
 * @param options - Options controlling the visibility tracking.
 * @returns Whether the element has been visible at least once.
 */
export function useHasBeenVisible(
  ref: RefObject<Element | null>,
  { enabled = true, rootMargin = '0px' }: UseHasBeenVisibleOptions = {},
): boolean {
  const [hasBeenVisible, setHasBeenVisible] = useState(
    () => !enabled || typeof IntersectionObserver === 'undefined',
  );

  useEffect(() => {
    if (hasBeenVisible) {
      return;
    }
    if (!enabled || typeof IntersectionObserver === 'undefined') {
      setHasBeenVisible(true);
      return;
    }
    const element = ref.current;
    if (!element) {
      return;
    }

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) {
          setHasBeenVisible(true);
          observer.disconnect();
        }
      },
      { rootMargin },
    );
    observer.observe(element);
    return () => observer.disconnect();
  }, [enabled, hasBeenVisible, ref, rootMargin]);

  return hasBeenVisible;
}
