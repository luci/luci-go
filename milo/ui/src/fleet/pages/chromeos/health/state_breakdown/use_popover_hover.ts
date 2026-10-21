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

import { useCallback, useEffect, useRef, useState } from 'react';

export interface VirtualElement {
  getBoundingClientRect: () => DOMRect;
  contextElement?: Element;
}

export interface UsePopoverHoverOptions {
  readonly enabled?: boolean;
  readonly enterDelay?: number;
  readonly leaveDelay?: number;
}

export interface UsePopoverHoverResult {
  readonly anchorEl: HTMLElement | VirtualElement | null;
  readonly open: boolean;
  readonly handleOpen: (
    target: HTMLElement,
    clientX?: number,
    delay?: number,
  ) => void;
  readonly handleClose: () => void;
  readonly cancelClose: () => void;
  readonly handleCloseImmediately: () => void;
}

/**
 * Reusable hook managing hover intent delays, locked initial cursor positioning,
 * and automatic dismissal on window scroll/wheel.
 */
export const usePopoverHover = ({
  enabled = true,
  enterDelay = 120,
  leaveDelay = 200,
}: UsePopoverHoverOptions = {}): UsePopoverHoverResult => {
  const [anchorEl, setAnchorEl] = useState<HTMLElement | VirtualElement | null>(
    null,
  );
  const openTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const closeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const handleCloseImmediately = useCallback(() => {
    if (openTimerRef.current) {
      clearTimeout(openTimerRef.current);
      openTimerRef.current = null;
    }
    if (closeTimerRef.current) {
      clearTimeout(closeTimerRef.current);
      closeTimerRef.current = null;
    }
    setAnchorEl(null);
  }, []);

  const cancelClose = useCallback(() => {
    if (closeTimerRef.current) {
      clearTimeout(closeTimerRef.current);
      closeTimerRef.current = null;
    }
  }, []);

  const handleOpen = useCallback(
    (target: HTMLElement, clientX?: number, delay = enterDelay) => {
      cancelClose();
      if (openTimerRef.current) {
        clearTimeout(openTimerRef.current);
        openTimerRef.current = null;
      }

      if (!enabled) return;

      const applyAnchor = () => {
        if (typeof clientX === 'number' && clientX > 0) {
          const barRect = target.getBoundingClientRect();
          setAnchorEl({
            getBoundingClientRect: () =>
              new DOMRect(clientX, barRect.top, 0, barRect.height),
            contextElement: target,
          });
        } else {
          setAnchorEl(target);
        }
      };

      if (delay > 0) {
        openTimerRef.current = setTimeout(applyAnchor, delay);
      } else {
        applyAnchor();
      }
    },
    [cancelClose, enabled, enterDelay],
  );

  const handleClose = useCallback(() => {
    if (openTimerRef.current) {
      clearTimeout(openTimerRef.current);
      openTimerRef.current = null;
    }
    if (closeTimerRef.current) {
      clearTimeout(closeTimerRef.current);
    }
    closeTimerRef.current = setTimeout(() => {
      setAnchorEl(null);
    }, leaveDelay);
  }, [leaveDelay]);

  // Clean up timers on unmount
  useEffect(() => {
    return () => {
      if (openTimerRef.current) {
        clearTimeout(openTimerRef.current);
      }
      if (closeTimerRef.current) {
        clearTimeout(closeTimerRef.current);
      }
    };
  }, []);

  const open = Boolean(anchorEl && enabled);

  // Close the popover immediately when the user scrolls or wheels,
  // unless scrolling inside the popover.
  useEffect(() => {
    if (!open) return;

    const handleScrollOrWheel = (e: Event) => {
      const target = e.target;
      if (target instanceof HTMLElement && target.closest('.MuiPopper-root')) {
        return;
      }
      handleCloseImmediately();
    };

    window.addEventListener('wheel', handleScrollOrWheel, {
      capture: true,
      passive: true,
    });
    window.addEventListener('touchmove', handleScrollOrWheel, {
      capture: true,
      passive: true,
    });
    window.addEventListener('scroll', handleScrollOrWheel, {
      capture: true,
      passive: true,
    });

    return () => {
      window.removeEventListener('wheel', handleScrollOrWheel, {
        capture: true,
      });
      window.removeEventListener('touchmove', handleScrollOrWheel, {
        capture: true,
      });
      window.removeEventListener('scroll', handleScrollOrWheel, {
        capture: true,
      });
    };
  }, [open, handleCloseImmediately]);

  return {
    anchorEl,
    open,
    handleOpen,
    handleClose,
    cancelClose,
    handleCloseImmediately,
  };
};
