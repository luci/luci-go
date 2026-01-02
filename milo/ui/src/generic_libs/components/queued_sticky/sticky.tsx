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

import { Box, SxProps, Theme } from '@mui/material';
import { ReactNode, useEffect, useRef, useState } from 'react';

import { useSizeRecorder } from './context';

export interface StickyProps {
  /**
   * If true, sticks to the top of the scrolling base.
   */
  readonly top?: boolean;
  /**
   * If true, sticks to the right of the scrolling base.
   */
  readonly right?: boolean;
  /**
   * If true, sticks to the bottom of the scrolling base.
   */
  readonly bottom?: boolean;
  /**
   * If true, sticks to the left of the scrolling base.
   */
  readonly left?: boolean;
  readonly sx?: SxProps<Theme>;
  /**
   * The content to display in the standard flow.
   *
   * Note: In Dual-Display mode (when `collapsedContent` is provided), this is rendered
   * as a separate sibling to the sticky layer. This means that layouts relying
   * on a single child (e.g., `flex` with `gap` on the parent) may not work as
   * expected.
   */
  readonly children: ReactNode;
  /**
   * Optional content to display when the component becomes "stuck".
   * If provided, the component enters "Dual-Display" mode: `children` are shown
   * in the normal flow, and `collapsedContent` is shown in a sticky overlay once stuck.
   */
  readonly collapsedContent?: ReactNode;
}

/**
 * Create a stick component that does not get overlapped by other `<Sticky />`
 * components in a sibling `<StickyOffset />`. See [the documentation](./doc.md)
 * for examples.
 *
 * It supports two modes:
 * 1. **Standard Sticky**: Only `children` are provided. The component behaves
 *    like a standard `position: sticky` wrapper.
 * 2. **Dual-Display**: Both `children` and `collapsedContent` are provided. The expanded
 *    state (`children`) scrolls naturally, and the collapsed state (`collapsedContent`)
 *    becomes sticky at the top once the expanded state has scrolled past.
 *
 * NOTE: In Dual-Display mode, this component renders TWO top-level elements
 * (a sticky anchor and a relative container). Parent layouts that expect a
 * single child (like `flex` with `gap`) will see both elements and may
 * display unexpected spacing.
 */
export function Sticky({
  top,
  right,
  bottom,
  left,
  sx,
  children,
  collapsedContent,
}: StickyProps) {
  const componentRef = useRef(undefined);
  const anchorRef = useRef<HTMLElement>(null);
  const expandedRef = useRef<HTMLElement>(null);
  const collapsedRef = useRef<HTMLElement>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);
  const sizeRecorder = useSizeRecorder();

  const [isStuck, setIsStuck] = useState(false);
  const [expandedSize, setExpandedSize] = useState({ width: 0, height: 0 });
  const [collapsedSize, setCollapsedSize] = useState({ width: 0, height: 0 });

  // 1. DYNAMIC SIZE MEASUREMENT
  // Subsequent sticky components in the same `StickyOffset` context need to know
  // where to position themselves (their `top`, `left`, etc. CSS values).
  // We use ResizeObserver to track the dimensions of both the expanded (natural flow)
  // and collapsed (sticky overlay) layers because their content or screen size
  // can change dynamically during the lifecycle of the page.
  useEffect(() => {
    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const { inlineSize: width, blockSize: height } = entry.borderBoxSize[0];
        if (entry.target === expandedRef.current) {
          setExpandedSize({ width, height });
        } else if (entry.target === collapsedRef.current) {
          setCollapsedSize({ width, height });
        }
      }
    });

    if (expandedRef.current) {
      observer.observe(expandedRef.current);
      const { width, height } = expandedRef.current.getBoundingClientRect();
      setExpandedSize({ width, height });
    }
    if (collapsedRef.current) {
      observer.observe(collapsedRef.current);
      const { width, height } = collapsedRef.current.getBoundingClientRect();
      setCollapsedSize({ width, height });
    }

    return () => observer.disconnect();
  }, [collapsedContent]);

  // 2. ACTIVE SIZE REPORTING
  // In "Dual-Display" mode (where an expanded header collapses when stuck):
  // - While expanded: The header scrolls with the page and isn't "blocking"
  //   subsequent headers yet. We report a height of 0 so following elements
  //   calculate their initial positions correctly relative to the parent container.
  // - While stuck: The header is now fixed at the top. We must report its collapsed
  //   height so that subsequent sticky elements stack below it instead of overlapping.
  useEffect(() => {
    let activeSize = expandedSize;
    if (collapsedContent && top) {
      activeSize = isStuck ? collapsedSize : { width: 0, height: 0 };
    }

    if (top) sizeRecorder.recordSize(componentRef, 'top', activeSize.height);
    if (right) sizeRecorder.recordSize(componentRef, 'right', activeSize.width);
    if (bottom)
      sizeRecorder.recordSize(componentRef, 'bottom', activeSize.height);
    if (left) sizeRecorder.recordSize(componentRef, 'left', activeSize.width);

    return () => sizeRecorder.remove(componentRef);
  }, [
    sizeRecorder,
    top,
    right,
    bottom,
    left,
    isStuck,
    expandedSize,
    collapsedSize,
    collapsedContent,
  ]);

  // 3. SENTINEL-BASED STUCK DETECTION
  // Standard CSS `position: sticky` does not provide an event for when an element
  // becomes stuck. We use a tiny, invisible "sentinel" div at the bottom of the
  // children. When this sentinel crosses the threshold (defined by the
  // accumulated offsets from all headers above this one), we toggle the `isStuck` state.
  useEffect(() => {
    if (
      !collapsedContent ||
      !top ||
      !sentinelRef.current ||
      !anchorRef.current
    ) {
      return;
    }

    const getResolvedTopOffset = () => {
      const computedStyle = getComputedStyle(anchorRef.current!);
      const topValue = computedStyle.top;
      const parsed = parseInt(topValue, 10);
      return isNaN(parsed) ? 0 : parsed;
    };

    const topOffsetPx = getResolvedTopOffset();

    const observer = new IntersectionObserver(
      ([entry]) => {
        // Toggle stuck state based on whether the bottom of the expanded content
        // has scrolled past our designated sticky offset line.
        setIsStuck(!entry.isIntersecting);
      },
      {
        threshold: 0,
        rootMargin: `-${topOffsetPx}px 0px 0px 0px`,
      },
    );

    observer.observe(sentinelRef.current);

    return () => observer.disconnect();
  }, [collapsedContent, top, anchorRef]);

  // Calculate CSS offsets based on values provided by the `StickyOffset` parent.
  const offsets: { [key: string]: string } = {};
  if (top) offsets['top'] = 'var(--accumulated-top)';
  if (right) offsets['right'] = 'var(--accumulated-right)';
  if (bottom) offsets['bottom'] = 'var(--accumulated-bottom)';
  if (left) offsets['left'] = 'var(--accumulated-left)';

  // RENDER LOGIC
  // We support two primary modes to handle both simple sticky elements and those
  // that need a separate, more compact UI state when stuck.

  // Mode 1: Dual-Display (Header collapses when stuck)
  // This mode uses a zero-height sticky box as an anchor. This prevents the
  // expanded "natural" layout from being pushed down by its own sticky version,
  // allowing a smooth transition where the collapsed header appears to
  // "emerge" as the expanded one scrolls away.
  if (collapsedContent && top) {
    return (
      <>
        {/* 1. STICKY ANCHOR LAYER
          - Stays sticky for the full duration of the scroll.
          - height: 0 ensures it doesn't displace siblings in natural flow.
        */}
        <Box
          ref={anchorRef}
          sx={{
            position: 'sticky',
            ...offsets,
            height: 0,
            zIndex: 1200,
            width: '100%',
            overflow: 'visible',
            ...sx,
            marginBottom: 0,
            padding: 0,
          }}
        >
          {/* 2. STICKY CONTENT
            - Absolutely positioned within the anchor layer.
            - Fades in/out based on the stuck state.
          */}
          <Box
            ref={collapsedRef}
            sx={{
              position: 'absolute',
              top: 0,
              left: 0,
              width: '100%',
              opacity: isStuck ? 1 : 0,
              visibility: isStuck ? 'visible' : 'hidden',
              pointerEvents: isStuck ? 'auto' : 'none',
              transition: 'opacity 0.2s ease-in-out',
            }}
          >
            {collapsedContent}
          </Box>
        </Box>

        {/* 3. EXPANDED LAYER (Normal Flow)
          - Contains the full content and the sentinel.
        */}
        <Box
          ref={expandedRef}
          sx={{
            position: 'relative',
            zIndex: 1,
          }}
        >
          {children}

          <div
            ref={sentinelRef}
            style={{
              position: 'absolute',
              bottom: 0,
              left: 0,
              width: '100%',
              height: '1px',
              visibility: 'hidden',
              pointerEvents: 'none',
            }}
          />
        </Box>
      </>
    );
  }

  // Mode 2: Standard Sticky
  // A standard position:sticky wrapper with no secondary content state.
  return (
    <Box
      sx={{
        ...sx,
        position: 'sticky',
        ...offsets,
      }}
      ref={expandedRef}
    >
      {children}
    </Box>
  );
}
