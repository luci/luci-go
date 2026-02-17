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

import { Box } from '@mui/material';
import { useVirtualizer } from '@tanstack/react-virtual';
import { useEffect, useRef } from 'react';

import { AggregationTreeItem } from './aggregation_tree_item';
import { useAggregationViewContext } from './context/context';

export function TestAggregationVirtualTree() {
  const { flattenedItems, scrollRequest } = useAggregationViewContext();
  const parentRef = useRef<HTMLDivElement>(null);

  const rowVirtualizer = useVirtualizer({
    count: flattenedItems.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 80,
    overscan: 5,
  });

  const lastScrolledTsRef = useRef<number>(0);

  useEffect(() => {
    if (
      scrollRequest &&
      scrollRequest.ts > lastScrolledTsRef.current &&
      flattenedItems.length > 0
    ) {
      const index = flattenedItems.findIndex(
        (node) => node.id === scrollRequest.id,
      );
      if (index !== -1) {
        rowVirtualizer.scrollToIndex(index, { align: 'center' });
        lastScrolledTsRef.current = scrollRequest.ts;
      }
    }
  }, [scrollRequest, flattenedItems, rowVirtualizer]);

  return (
    <Box
      ref={parentRef}
      sx={{
        height: '100%',
        overflow: 'auto',
      }}
    >
      <div
        style={{
          height: `${rowVirtualizer.getTotalSize()}px`,
          width: '100%',
          position: 'relative',
        }}
      >
        <div
          style={{
            transform: `translateY(${rowVirtualizer.getVirtualItems()[0]?.start ?? 0}px)`,
          }}
        >
          {rowVirtualizer.getVirtualItems().map((virtualDesc) => {
            const node = flattenedItems[virtualDesc.index];
            return (
              <AggregationTreeItem
                key={node.id}
                node={node}
                measureRef={rowVirtualizer.measureElement}
              />
            );
          })}
        </div>
      </div>
    </Box>
  );
}
