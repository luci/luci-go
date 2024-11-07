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
import { ElementType, ReactNode } from 'react';

import { useDepth, useOffsets, QueuedStickyContextProvider } from './context';

export interface StickyOffsetProps {
  readonly component?: ElementType;
  readonly sx?: SxProps<Theme>;
  readonly children: ReactNode;
}

/**
 * Ensures child `<Sticky />` components don't overlap sibling `<Sticky />`
 * components. See [the documentation](./doc.md) for examples.
 */
export function StickyOffset({
  component = 'div',
  sx,
  children,
}: StickyOffsetProps) {
  const offsets = useOffsets();
  const depth = useDepth();
  const parentDepth = depth - 1;
  return (
    <Box
      component={component}
      sx={{
        ...sx,

        // Add the new offsets to the offsets in the parent layer.
        [`--accumulated-top-${depth}`]: `calc(var(--accumulated-top-${parentDepth}) + ${offsets.top}px)`,
        [`--accumulated-right-${depth}`]: `calc(var(--accumulated-right-${parentDepth}) + ${offsets.right}px)`,
        [`--accumulated-bottom-${depth}`]: `calc(var(--accumulated-bottom-${parentDepth}) + ${offsets.bottom}px)`,
        [`--accumulated-left-${depth}`]: `calc(var(--accumulated-left-${parentDepth}) + ${offsets.left}px)`,

        // Update the stable CSS variables to point to the appropriate offsets
        // in the current layer.
        [`--accumulated-top`]: `var(--accumulated-top-${depth})`,
        [`--accumulated-right`]: `var(--accumulated-right-${depth})`,
        [`--accumulated-bottom`]: `var(--accumulated-bottom-${depth})`,
        [`--accumulated-left`]: `var(--accumulated-left-${depth})`,
      }}
    >
      <QueuedStickyContextProvider>{children}</QueuedStickyContextProvider>
    </Box>
  );
}
