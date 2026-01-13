// Copyright 2025 The LUCI Authors.
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

import { Checkbox, colors, MenuItem } from '@mui/material';
import { useVirtualizer } from '@tanstack/react-virtual';
import React, { useRef } from 'react';

import { BLANK_VALUE } from '@/fleet/constants/filters';
import { OptionValue } from '@/fleet/types/option';
import { keyboardListNavigationHandler } from '@/fleet/utils';
import { SortedElement } from '@/fleet/utils/fuzzy_sort';

import { HighlightCharacter } from '../highlight_character';

interface OptionsMenuProps {
  elements: SortedElement<OptionValue>[];
  selectedElements: Set<string>;
  flipOption: (value: string) => void;
  onNavigateUp?: (e: React.KeyboardEvent) => void;
  onNavigateDown?: (e: React.KeyboardEvent) => void;
}

export const OptionsMenu = ({
  elements,
  selectedElements,
  flipOption,
  onNavigateUp,
  onNavigateDown,
}: OptionsMenuProps) => {
  const parentRef = useRef(null);

  const virtualizer = useVirtualizer({
    count: elements.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 36, // Increased slightly to match Material UI MenuItem height typically
    enabled: true,
    overscan: 20,
    getItemKey: (index) => elements[index].el.value, // Provide explicit key for virtualizer cache
  });

  const virtualRows = virtualizer.getVirtualItems();

  if (elements.length === 0) {
    return (
      <div
        css={{
          display: 'flex',
          alignItems: 'center',
          padding: '8px',
          justifyContent: 'center',
        }}
      >
        <span>No options available</span>
      </div>
    );
  }

  return (
    <div
      ref={parentRef}
      css={{
        overflow: 'auto',
        maxHeight: 'inherit',
        width: '100%',
      }}
    >
      <div
        css={{
          width: '100%',
          height: `${virtualizer.getTotalSize()}px`,
          position: 'relative',
        }}
      >
        <div
          style={{
            transform: `translateY(${virtualRows[0]?.start ?? 0}px)`,
            width: '100%',
            position: 'absolute',
            top: 0,
            left: 0,
          }}
        >
          {virtualRows.map((virtualRow) => {
            const item = elements[virtualRow.index];
            return (
              <MenuItem
                key={item.el.value}
                data-index={virtualRow.index}
                ref={virtualizer.measureElement}
                disableRipple
                onClick={(e) => {
                  if (e.type === 'keydown' || e.type === 'keyup') {
                    const parsedE =
                      e as unknown as React.KeyboardEvent<HTMLLIElement>;
                    if (parsedE.key === ' ') return;
                    if (parsedE.key === 'Enter' && parsedE.ctrlKey) return;
                  }
                  flipOption(elements[virtualRow.index].el.value);
                }}
                onKeyDown={(e) =>
                  keyboardListNavigationHandler(
                    e,
                    virtualRow.index === virtualRows.length - 1 &&
                      onNavigateDown
                      ? () => onNavigateDown(e)
                      : undefined,
                    virtualRow.index === 0 && onNavigateUp
                      ? () => onNavigateUp(e)
                      : undefined,
                  )
                }
                sx={{
                  display: 'flex',
                  alignItems: 'center',
                  width: '100%',
                  padding: '6px 12px',
                  ...(item.el.label === BLANK_VALUE && {
                    color: colors.grey[500],
                  }),
                }}
              >
                <Checkbox
                  sx={{
                    padding: 0,
                    marginRight: '13px',
                  }}
                  size="small"
                  checked={
                    selectedElements.has(elements[virtualRow.index].el.value) ??
                    false
                  }
                  tabIndex={-1}
                />
                <HighlightCharacter
                  variant="body2"
                  highlightIndexes={item.matches}
                  sx={{
                    whiteSpace: 'nowrap',
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    display: 'block',
                  }}
                >
                  {item.el.label}
                </HighlightCharacter>
              </MenuItem>
            );
          })}
        </div>
      </div>
    </div>
  );
};
