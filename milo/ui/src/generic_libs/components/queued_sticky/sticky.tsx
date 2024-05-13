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
import { ReactNode, useEffect, useRef } from 'react';

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
  readonly children: ReactNode;
}

/**
 * Create a stick component that does not get overlapped by other `<Sticky />`
 * components in a sibling `<StickyOffset />`. See [the documentation](./doc.md)
 * for examples.
 */
export function Sticky({
  top,
  right,
  bottom,
  left,
  sx,
  children,
}: StickyProps) {
  const componentRef = useRef();
  const containerEleRef = useRef<HTMLElement>();
  const sizeRecorder = useSizeRecorder();

  useEffect(() => {
    const updateSize = (width: number, height: number) => {
      if (top) {
        sizeRecorder.recordSize(componentRef, 'top', height);
      }
      if (right) {
        sizeRecorder.recordSize(componentRef, 'right', width);
      }
      if (bottom) {
        sizeRecorder.recordSize(componentRef, 'bottom', height);
      }
      if (left) {
        sizeRecorder.recordSize(componentRef, 'left', width);
      }
    };

    // Set the initial size.
    const containerEle = containerEleRef.current!;
    const { width, height } = containerEle.getBoundingClientRect();
    updateSize(width, height);

    // React to size changes.
    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        switch (entry.target) {
          case containerEle:
            updateSize(
              entry.borderBoxSize[0].inlineSize,
              entry.borderBoxSize[0].blockSize,
            );
            break;
          default:
            break;
        }
      }
    });
    observer.observe(containerEle);

    return () => {
      // Remove the element from size calculation.
      observer.disconnect();
      sizeRecorder.remove(componentRef);
    };
  }, [sizeRecorder, top, right, bottom, left]);

  // The caller may decide to add their own offsets.
  // Only override the offset when the direction is specified.
  const offsets: { [key: string]: string } = {};
  if (top) {
    offsets['top'] = 'var(--accumulated-top)';
  }
  if (right) {
    offsets['right'] = 'var(--accumulated-right)';
  }
  if (bottom) {
    offsets['bottom'] = 'var(--accumulated-bottom)';
  }
  if (left) {
    offsets['left'] = 'var(--accumulated-left)';
  }

  return (
    <Box
      sx={{
        ...sx,
        position: 'sticky',
        ...offsets,
      }}
      ref={containerEleRef}
    >
      {children}
    </Box>
  );
}
