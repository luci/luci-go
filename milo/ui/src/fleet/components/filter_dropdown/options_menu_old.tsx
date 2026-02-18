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
import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
} from 'react';

import { OptionValue } from '@/fleet/types/option';
import { keyboardListNavigationHandler } from '@/fleet/utils';
import { SortedElement } from '@/fleet/utils/fuzzy_sort';

import { EllipsisTooltip } from '../ellipsis_tooltip';
import { HighlightCharacter } from '../highlight_character';
interface OptionsMenuProps {
  elements: SortedElement<OptionValue>[];
  selectedElements: Set<string>;
  flipOption: (value: string) => void;
  onNavigateUp?: (e: React.KeyboardEvent<HTMLLIElement>) => void;
}

/**
 * @deprecated This component will be removed when all pages are migrated to go/fleet-console-unified-filter-bar
 */
export const OptionsMenuOld = forwardRef(function OptionsMenuOld(
  { elements, selectedElements, flipOption, onNavigateUp }: OptionsMenuProps,
  ref,
) {
  const parentRef = useRef<HTMLDivElement>(null);
  const virtualizer = useVirtualizer({
    count: elements.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 32,
    enabled: true,
    overscan: 20,
  });

  const [shouldFocus, setShouldFocus] = useState(false);

  useImperativeHandle(ref, () => ({
    focus: () => {
      setShouldFocus(true);
    },
  }));

  const virtualRows = virtualizer.getVirtualItems();

  useEffect(() => {
    if (shouldFocus && virtualRows.length > 0) {
      const firstItem = parentRef.current?.querySelector(
        '[data-index="0"]',
      ) as HTMLElement;
      if (firstItem) {
        firstItem.focus();
        setShouldFocus(false);
      }
    }
  }, [shouldFocus, virtualRows]);

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
        overflowY: 'hidden',
        overflow: 'auto',
        maxHeight: 'inherit',
        minWidth: '100%',
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
                onKeyDown={(e) => {
                  if (
                    e.key === 'ArrowUp' &&
                    virtualRow.index === 0 &&
                    onNavigateUp
                  ) {
                    onNavigateUp(e as React.KeyboardEvent<HTMLLIElement>);
                  } else {
                    keyboardListNavigationHandler(e);
                  }
                }}
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
                <EllipsisTooltip tooltip={item.el.label}>
                  <HighlightCharacter
                    variant="body2"
                    highlightIndexes={item.matches}
                    css={{
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                    }}
                  >
                    {item.el.label}
                  </HighlightCharacter>
                </EllipsisTooltip>
              </MenuItem>
            );
          })}
        </div>
      </div>
    </div>
  );
});
