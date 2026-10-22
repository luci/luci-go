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

import {
  ClickAwayListener,
  MenuProps,
  Paper,
  PopoverOrigin,
  Popper,
  PopperPlacementType,
  PopperProps,
  SxProps,
} from '@mui/material';
import {
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type MouseEventHandler,
  type ReactNode,
  CSSProperties,
} from 'react';
import { flushSync } from 'react-dom';

import { hasAnyModifier, keyboardListNavigationHandler } from '../../utils';
import { shouldIgnoreClickAway } from '../../utils/click_away';
import { SearchInput } from '../search_input';

import { Footer } from './footer';

type OptionsDropdownProps = Omit<MenuProps, 'open' | 'maxHeight'> & {
  onClose: (event?: object, reason?: 'backdropClick' | 'escapeKeyDown') => void;
  onApply: () => void;
  renderChild: (
    searchQuery: string,
    onNavigateUp: (e: KeyboardEvent) => void,
  ) => ReactNode;
  anchorEl: HTMLElement | null;
  open: boolean;
  anchorOrigin?: PopoverOrigin | undefined;
  transformOrigin?: PopoverOrigin | undefined;
  onKeyDown?: React.KeyboardEventHandler<HTMLDivElement>;
  onKeyDownCapture?: React.KeyboardEventHandler<HTMLDivElement>;
  enableSearchInput?: boolean;
  autoFocusSearchInput?: boolean;
  disableClickAway?: boolean;
  disableTypeToSearch?: boolean;
  externalSearchQuery?: string;
  controlledSearchQuery?: string;
  onSearchQueryChange?: (searchQuery: string) => void;
  resetKey?: unknown;
  maxHeight?: CSSProperties['maxHeight'];
  onResetClick?: MouseEventHandler<HTMLButtonElement>;
  onResetWidthsClick?: MouseEventHandler<HTMLButtonElement>;
  footerButtons?: ('reset' | 'cancel' | 'apply' | 'reset-widths')[];
  disablePortal?: boolean;
  disableEnforceFocus?: boolean;
  disableRestoreFocus?: boolean;
  hideBackdrop?: boolean;
  sx?: SxProps;
  disableScrollUpdate?: boolean;
  closeOnScroll?: boolean;
};

const mapOriginToPlacement = (
  anchorOrigin: PopoverOrigin,
  transformOrigin?: PopoverOrigin,
): PopperPlacementType => {
  const { vertical, horizontal } = anchorOrigin;

  if (vertical === 'bottom') {
    if (horizontal === 'center') return 'bottom';
    if (horizontal === 'right') return 'bottom-end';
    return 'bottom-start';
  }
  if (vertical === 'top') {
    if (horizontal === 'center') return 'top';
    // If transformOrigin is configured to push it to the side, we might want left/right.
    // But for simplicity, we map top-right to top-end or right-start?
    // Based on FilterItem usage (side menu), if anchor is top-right, it likely wants right-start.
    if (horizontal === 'right') {
      if (transformOrigin?.horizontal === 'left') return 'right-start';
      return 'top-end';
    }
    if (horizontal === 'left') {
      if (transformOrigin?.horizontal === 'right') return 'left-start';
      return 'top-start';
    }
  }
  return 'bottom-start';
};

export function OptionsDropdown({
  onClose,
  onApply,
  anchorEl,
  open,
  renderChild,
  anchorOrigin = {
    vertical: 'top',
    horizontal: 'right',
  },
  transformOrigin,
  onKeyDown,
  onKeyDownCapture,
  enableSearchInput = false,
  autoFocusSearchInput = true,
  disableClickAway = false,
  disableTypeToSearch = false,
  externalSearchQuery = '',
  controlledSearchQuery,
  onSearchQueryChange,
  resetKey,
  maxHeight = 'auto',
  onResetClick,
  onResetWidthsClick,
  footerButtons = ['apply', 'cancel'],
  disablePortal,
  sx,
  closeOnScroll = false,
  disableScrollUpdate = false,
}: OptionsDropdownProps) {
  const searchInput = useRef<HTMLInputElement>(null);
  const listContainerRef = useRef<HTMLDivElement>(null);
  const paperRef = useRef<HTMLDivElement>(null);
  const popperRef = useRef<{ update: () => void } | null>(null);
  const [searchQuery, setSearchQuery] = useState(externalSearchQuery);
  const historyRef = useRef<{ entries: string[]; index: number }>({
    entries: [externalSearchQuery],
    index: 0,
  });

  const placement = mapOriginToPlacement(anchorOrigin, transformOrigin);
  const effectiveSearchQuery = controlledSearchQuery ?? searchQuery;

  const updateSearchQuery = (nextQuery: string) => {
    const history = historyRef.current;
    if (history.entries[history.index] !== nextQuery) {
      const nextEntries = history.entries.slice(0, history.index + 1);
      nextEntries.push(nextQuery);
      historyRef.current = {
        entries: nextEntries,
        index: nextEntries.length - 1,
      };
    }
    setSearchQuery(nextQuery);
    onSearchQueryChange?.(nextQuery);
  };

  const handleUndoRedoKeyDown = (e: KeyboardEvent<HTMLElement>): boolean => {
    if (!(e.ctrlKey || e.metaKey) || e.altKey) {
      return false;
    }
    const key = e.key.toLowerCase();
    const isUndo = key === 'z' && !e.shiftKey;
    const isRedo = (key === 'y' && !e.shiftKey) || (key === 'z' && e.shiftKey);
    if (!isUndo && !isRedo) {
      return false;
    }

    const history = historyRef.current;
    const targetIndex = isUndo ? history.index - 1 : history.index + 1;
    if (targetIndex >= 0 && targetIndex < history.entries.length) {
      history.index = targetIndex;
      const nextVal = history.entries[targetIndex];
      setSearchQuery(nextVal);
      onSearchQueryChange?.(nextVal);
    }
    e.preventDefault();
    e.stopPropagation();
    return true;
  };

  const prevResetKeyRef = useRef(resetKey);
  useEffect(() => {
    const isReset = prevResetKeyRef.current !== resetKey;
    prevResetKeyRef.current = resetKey;
    if (
      isReset ||
      externalSearchQuery !==
        historyRef.current.entries[historyRef.current.index]
    ) {
      setSearchQuery(externalSearchQuery);
      historyRef.current = {
        entries: [externalSearchQuery],
        index: 0,
      };
    }
  }, [resetKey, externalSearchQuery]);

  useEffect(() => {
    if (!open || !paperRef.current || typeof ResizeObserver === 'undefined') {
      return;
    }
    const observer = new ResizeObserver(() => {
      popperRef.current?.update();
    });
    observer.observe(paperRef.current);
    return () => observer.disconnect();
  }, [open, resetKey]);

  useEffect(() => {
    if (!open || !anchorEl || typeof ResizeObserver === 'undefined') return;
    const updatePosition = () => {
      popperRef.current?.update();
    };
    const resizeObserver = new ResizeObserver(updatePosition);
    resizeObserver.observe(document.body);
    if (anchorEl.parentElement) {
      resizeObserver.observe(anchorEl.parentElement);
    }
    return () => resizeObserver.disconnect();
  }, [open, anchorEl]);

  useEffect(() => {
    if (enableSearchInput) {
      // Small timeout to allow the popover to mount and settle
      if (open) {
        if (autoFocusSearchInput) {
          setTimeout(() => {
            searchInput.current?.focus();
          }, 0);
        }
      } else {
        setSearchQuery('');
        historyRef.current = {
          entries: [''],
          index: 0,
        };
      }
    }
  }, [open, enableSearchInput, autoFocusSearchInput]);

  useEffect(() => {
    if (!open || !closeOnScroll) return;

    const handleScroll = (event: Event) => {
      const target = event.target;

      // Don't close if the scroll event originates from inside the dropdown.
      // We need to check if target is an Element, as it can be the document itself.
      if (
        target instanceof Element &&
        target.closest('.fc-dropdown-container')
      ) {
        return;
      }

      if (onClose) {
        flushSync(() => {
          onClose(event, 'backdropClick');
        });
      }
    };

    window.addEventListener('scroll', handleScroll, {
      capture: true,
      passive: true,
    });
    return () => {
      window.removeEventListener('scroll', handleScroll, { capture: true });
    };
  }, [open, onClose, closeOnScroll]);

  return (
    <Popper
      popperRef={popperRef as PopperProps['popperRef']}
      disablePortal={disablePortal}
      open={open}
      anchorEl={anchorEl}
      placement={placement}
      modifiers={[
        {
          name: 'eventListeners',
          options: { scroll: !disableScrollUpdate, resize: true },
        },
        {
          name: 'flip',
          enabled: !disableScrollUpdate,
          options: {
            boundary: 'viewport',
          },
        },
        {
          name: 'preventOverflow',
          enabled: !disableScrollUpdate,
          options: {
            boundary: 'viewport',
          },
        },
        {
          name: 'offset',
          options: {
            offset: [0, 4],
          },
        },
      ]}
      sx={[
        {
          zIndex: 1401, // luci's cookie_consent_bar is 1400
        },
        ...(sx ? (Array.isArray(sx) ? sx : [sx]) : []),
      ]}
    >
      <ClickAwayListener
        onClickAway={(e) => {
          if (disableClickAway) {
            return;
          }
          if (shouldIgnoreClickAway(e, anchorEl, paperRef.current)) {
            return;
          }
          if (onClose) onClose(e, 'backdropClick');
        }}
      >
        <Paper
          ref={paperRef}
          elevation={2}
          className="fc-dropdown-container"
          onClick={(e) => e.stopPropagation()}
          onKeyDownCapture={onKeyDownCapture}
          onKeyDown={(e) => {
            e.stopPropagation();

            if (e.key === 'Escape') {
              if (onClose) onClose(e, 'escapeKeyDown');
            }

            if (e.key === 'Enter' && e.ctrlKey) {
              onApply();
            }

            if (enableSearchInput) {
              if (handleUndoRedoKeyDown(e)) {
                return;
              }
            }

            if (enableSearchInput && !disableTypeToSearch) {
              keyboardListNavigationHandler(e);
              switch (e.key) {
                case 'Delete':
                case 'Cancel':
                case 'Backspace':
                  if (document.activeElement === searchInput.current) {
                    return;
                  }
                  updateSearchQuery('');
                  searchInput?.current?.focus();
              }
              // if the key is a single alphanumeric character without modifier
              if (/^[a-zA-Z0-9_.\-/]$/.test(e.key) && !hasAnyModifier(e)) {
                if (document.activeElement === searchInput.current) return;
                searchInput?.current?.focus();
                updateSearchQuery(effectiveSearchQuery + e.key);
                e.preventDefault(); // Avoid race condition to type twice in the input
              }
            }

            if (onKeyDown) onKeyDown(e);
          }}
          sx={{
            outline: 'none',
            display: 'flex',
            flexDirection: 'column',
            paddingTop: '8px',
            paddingBottom: 0,
            pointerEvents: 'auto',
            overscrollBehavior: 'contain',
            width: '100%',
            minWidth: 360,
          }}
        >
          {enableSearchInput && (
            <SearchInput
              key="options-search-input"
              searchInput={searchInput}
              searchQuery={effectiveSearchQuery}
              // eslint-disable-next-line jsx-a11y/no-autofocus
              autoFocus={autoFocusSearchInput}
              fullWidth
              onChange={(e) => {
                updateSearchQuery(e.currentTarget.value);
              }}
              onKeyDown={(e) => {
                if (handleUndoRedoKeyDown(e)) {
                  return;
                }
                if (e.key === 'ArrowDown' || (e.key === 'j' && e.ctrlKey)) {
                  const firstItem =
                    listContainerRef.current?.querySelector<HTMLElement>(
                      '[role="menuitem"], [role="option"]',
                    ) ??
                    listContainerRef.current?.querySelector<HTMLElement>(
                      'button',
                    );
                  firstItem?.focus();
                  e.preventDefault();
                  e.stopPropagation();
                  return;
                }
                if (
                  (e.ctrlKey || e.metaKey) &&
                  e.key !== 'k' &&
                  e.key !== 'Enter'
                ) {
                  e.stopPropagation();
                  return;
                }
                keyboardListNavigationHandler(e, undefined, () => {
                  if (onClose) onClose(e, 'escapeKeyDown');
                  e.preventDefault();
                  e.stopPropagation();
                });
              }}
            />
          )}
          <div
            css={{
              maxHeight: maxHeight,
              overflow: 'auto',
              paddingTop: enableSearchInput ? '8px' : 0,
              overscrollBehavior: 'contain',
            }}
            tabIndex={-1}
            role="menu"
            key="options-menu-container"
            ref={listContainerRef}
            onWheel={(e) => e.stopPropagation()}
            onTouchMove={(e) => e.stopPropagation()}
          >
            {renderChild(effectiveSearchQuery, (e) => {
              searchInput.current?.focus();
              e.stopPropagation();
              e.preventDefault();
            })}
          </div>
          {footerButtons && footerButtons.length > 0 && (
            <Footer
              footerButtons={footerButtons}
              onCancelClick={(e) => {
                // Treat cancel button as escape key down for focus purposes? Or explicit cancel?
                // Usually Cancel button just closes.
                if (onClose) onClose(e, 'escapeKeyDown');
              }}
              onApplyClick={onApply}
              onResetClick={onResetClick}
              onResetWidthsClick={onResetWidthsClick}
              key="options-menu-footer"
            />
          )}
        </Paper>
      </ClickAwayListener>
    </Popper>
  );
}
