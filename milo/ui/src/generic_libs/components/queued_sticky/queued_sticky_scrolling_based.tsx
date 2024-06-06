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
import { ElementType, ForwardedRef, ReactNode, forwardRef } from 'react';

import { QueuedStickyContextProvider, DepthCtx } from './context';

export interface QueuedStickyScrollingBaseProps {
  readonly component?: ElementType;
  readonly sx?: SxProps<Theme>;
  readonly className?: string;
  readonly children: ReactNode;
}

/**
 * Declares a scrolling base for `<Sticky />` components. This should be added
 * to the scrolling container under which `<Sticky />` is used. See
 * [the documentation](./doc.md) for examples.
 */
export const QueuedStickyScrollingBase = forwardRef(
  function QueuedStickyScrollingBase(
    { component, sx, className, children }: QueuedStickyScrollingBaseProps,
    ref: ForwardedRef<HTMLDivElement>,
  ) {
    return (
      <Box
        className={className}
        component={component}
        ref={ref}
        sx={{
          ...sx,

          // These CSS variables are used to compute the actual offsets.
          // They shall not be override by the child components otherwise the size
          // information will be lost.
          '--accumulated-top-0': '0px',
          '--accumulated-right-0': '0px',
          '--accumulated-bottom-0': '0px',
          '--accumulated-left-0': '0px',

          // Expose the accumulated offsets in the current layer under stable
          // CSS variable names so sticky components don't need to know the
          // current depth.
          '--accumulated-top': 'var(--accumulated-top-0)',
          '--accumulated-right': 'var(--accumulated-right-0)',
          '--accumulated-bottom': 'var(--accumulated-bottom-0)',
          '--accumulated-left': 'var(--accumulated-left-0)',
        }}
      >
        <DepthCtx.Provider value={0}>
          <QueuedStickyContextProvider>{children}</QueuedStickyContextProvider>
        </DepthCtx.Provider>
      </Box>
    );
  },
);
