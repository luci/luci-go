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

import { useEffect } from 'react';

// MenuScrollCloser is a helper component that listens for window scroll events
// and closes the parent menu. This is needed because Material React Table
// doesn't expose a way to disable scroll updating for the column actions menu,
// which can cause the menu to detach and reposition incorrectly (crashing to 0,0).
export function MenuScrollCloser({ closeMenu }: { closeMenu: () => void }) {
  useEffect(() => {
    const handleScroll = (event: Event) => {
      const target = event.target;
      // Don't close if scrolling inside the dropdown or column menu
      if (
        target instanceof Element &&
        (target.closest('.fc-dropdown-container') ||
          target.closest('.MuiMenu-paper'))
      ) {
        return;
      }
      event.stopPropagation(); // Stop event from reaching Popper to prevent flashing at 0,0
      closeMenu();
    };

    window.addEventListener('scroll', handleScroll, {
      capture: true,
      passive: true,
    });
    return () => {
      window.removeEventListener('scroll', handleScroll, { capture: true });
    };
  }, [closeMenu]);

  return null;
}
