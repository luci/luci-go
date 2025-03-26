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

import { Checkbox, MenuItem } from '@mui/material';
import { useVirtualizer } from '@tanstack/react-virtual';
import { useRef } from 'react';

import { OptionValue } from '@/fleet/types/option';
import { keyboardUpDownHandler } from '@/fleet/utils';
import { SortedElement } from '@/fleet/utils/fuzzy_sort';

import { HighlightCharacter } from '../highlight_character';

interface OptionsMenuProps {
  elements: SortedElement<OptionValue>[];
  selectedElements: Set<string>;
  flipOption: (value: string) => void;
}

export const OptionsMenu = ({
  elements,
  selectedElements,
  flipOption,
}: OptionsMenuProps) => {
  const parentRef = useRef(null);

  const virtualizer = useVirtualizer({
    count: elements.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 32,
    enabled: true,
    overscan: 20,
  });

  const virtualRows = virtualizer.getVirtualItems();

  return (
    <div
      ref={parentRef}
      css={{
        overflowY: 'hidden',
        overflow: 'auto',
        maxHeight: 'inherit',
        width: '100%',
      }}
    >
      <div
        css={{
          height: `${virtualizer.getTotalSize()}px`,
          width: '100%',
        }}
      >
        <div
          style={{
            width: '100%',
            transform: `translateY(${virtualRows[0]?.start ?? 0}px)`,
            overflowY: 'hidden',
          }}
        >
          {virtualRows.map((virtualRow) => {
            const item = elements[virtualRow.index];
            return (
              <MenuItem
                key={virtualRow.index}
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
                // eslint-disable-next-line jsx-a11y/no-autofocus
                autoFocus={virtualRow.index === 0}
                onKeyDown={keyboardUpDownHandler}
                css={{
                  display: 'flex',
                  alignItems: 'center',
                  padding: '6px 12px',
                  textOverflow: 'ellipsis',
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
                  css={{
                    overflow: 'hidden',
                    textWrap: 'wrap',
                    width: '100%',
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
