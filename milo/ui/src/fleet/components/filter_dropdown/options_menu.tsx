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

import { Button, Checkbox, colors, MenuItem } from '@mui/material';
import { useVirtualizer } from '@tanstack/react-virtual';
import React, { useRef, forwardRef, useImperativeHandle } from 'react';

import { BLANK_VALUE } from '@/fleet/constants/filters';
import { OptionValue } from '@/fleet/types/option';
import { SortedElement } from '@/fleet/utils/fuzzy_sort';

import { HighlightCharacter } from '../highlight_character';

interface OptionsMenuProps {
  elements: SortedElement<OptionValue>[];
  selectedElements: Set<string>;
  flipOption: (value: string) => void;
  selectOnly?: (value: string) => void;
  checkedIcon?: React.ReactNode;
}

export interface OptionsMenuHandle {
  focusFirst: () => void;
  focusLast: () => void;
}

export const OptionsMenu = forwardRef<OptionsMenuHandle, OptionsMenuProps>(
  function OptionsMenu(
    {
      elements,
      selectedElements,
      flipOption,
      selectOnly,
      checkedIcon,
    }: OptionsMenuProps,
    ref,
  ) {
    const parentRef = useRef<HTMLDivElement>(null);

    const virtualizer = useVirtualizer({
      count: elements.length,
      getScrollElement: () => parentRef.current,
      estimateSize: () => 36, // Increased slightly to match Material UI MenuItem height typically
      enabled: true,
      overscan: 20,
      getItemKey: (index) => elements[index].el.value, // Provide explicit key for virtualizer cache
    });

    const virtualRows = virtualizer.getVirtualItems();

    const focusIndexResilient = (index: number, retries = 5) => {
      virtualizer.scrollToIndex(index, { align: 'auto' });

      const tryFocus = (count: number) => {
        const el = parentRef.current?.querySelector<HTMLElement>(
          `[data-index="${index}"]`,
        );
        if (el) {
          el.focus();
        } else if (count > 0) {
          requestAnimationFrame(() => tryFocus(count - 1));
        }
      };

      requestAnimationFrame(() => tryFocus(retries));
    };

    useImperativeHandle(ref, () => ({
      focusFirst: () => {
        focusIndexResilient(0);
      },
      focusLast: () => {
        focusIndexResilient(elements.length - 1);
      },
    }));

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
        role="menu"
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
                  onKeyDown={(e) => {
                    const isDown =
                      e.key === 'ArrowDown' ||
                      (e.key === 'j' && (e.metaKey || e.ctrlKey));
                    const isUp =
                      e.key === 'ArrowUp' ||
                      (e.key === 'k' && (e.metaKey || e.ctrlKey));

                    if (isDown) {
                      const nextIndex = virtualRow.index + 1;
                      const targetIndex =
                        nextIndex >= elements.length ? 0 : nextIndex;
                      focusIndexResilient(targetIndex);
                      e.preventDefault();
                      e.stopPropagation();
                    }

                    if (isUp) {
                      const prevIndex = virtualRow.index - 1;
                      const targetIndex =
                        prevIndex < 0 ? elements.length - 1 : prevIndex;
                      focusIndexResilient(targetIndex);
                      e.preventDefault();
                      e.stopPropagation();
                    }
                  }}
                  sx={{
                    display: 'flex',
                    alignItems: 'center',
                    width: '100%',
                    padding: '6px 12px',
                    ...(item.el.label === BLANK_VALUE && {
                      fontStyle: 'italic',
                      color: colors.grey[700],
                    }),
                    ...(item.el.inScope === false && {
                      color: colors.grey[500],
                    }),
                    ...(item.el.isSignificant === false && {
                      color: colors.grey[500],
                    }),
                    '&:hover .only-button': {
                      opacity: 1,
                      visibility: 'visible',
                    },
                    '& .only-button': {
                      opacity: 0,
                      visibility: 'hidden',
                      transition: 'opacity 0.2s, visibility 0.2s',
                      marginLeft: 'auto',
                    },
                  }}
                >
                  <Checkbox
                    sx={{
                      padding: 0,
                      marginRight: '13px',
                    }}
                    size="small"
                    checked={
                      selectedElements.has(
                        elements[virtualRow.index].el.value,
                      ) ?? false
                    }
                    tabIndex={-1}
                    slotProps={{
                      input: {
                        'aria-label': item.el.label,
                      },
                    }}
                    checkedIcon={checkedIcon}
                  />
                  <HighlightCharacter
                    variant="body2"
                    highlightIndexes={
                      item.el.isSignificant === false ? [] : item.matches
                    }
                    sx={{
                      whiteSpace: 'nowrap',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      display: 'block',
                      flexGrow: 1,
                    }}
                  >
                    {item.el.label}
                  </HighlightCharacter>
                  {selectOnly && (
                    <Button
                      className="only-button"
                      size="small"
                      variant="text"
                      color="primary"
                      onClick={(e) => {
                        e.stopPropagation();
                        selectOnly(elements[virtualRow.index].el.value);
                      }}
                      sx={{
                        minWidth: 'unset',
                        padding: '2px 6px',
                        textTransform: 'none',
                        fontSize: '0.75rem',
                        fontWeight: 'bold',
                      }}
                    >
                      Only
                    </Button>
                  )}
                </MenuItem>
              );
            })}
          </div>
        </div>
      </div>
    );
  },
);
