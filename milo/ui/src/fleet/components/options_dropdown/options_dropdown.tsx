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
  enableSearchInput?: boolean;
  maxHeight?: CSSProperties['maxHeight'];
  onResetClick?: MouseEventHandler<HTMLButtonElement>;
  footerButtons?: ('reset' | 'cancel' | 'apply')[];
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
  enableSearchInput = false,
  maxHeight = 'auto',
  onResetClick,
  footerButtons = ['apply', 'cancel'],
  disablePortal,
  sx,
  closeOnScroll = false,
  disableScrollUpdate = false,
}: OptionsDropdownProps) {
  const searchInput = useRef<HTMLInputElement>(null);
  const listContainerRef = useRef<HTMLDivElement>(null);
  const [searchQuery, setSearchQuery] = useState('');

  const placement = mapOriginToPlacement(anchorOrigin, transformOrigin);

  useEffect(() => {
    if (enableSearchInput) {
      // Small timeout to allow the popover to mount and settle
      if (open) {
        setTimeout(() => {
          searchInput.current?.focus();
        }, 0);
      } else {
        setSearchQuery('');
      }
    }
  }, [open, enableSearchInput]);

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
      disablePortal={disablePortal}
      open={open}
      anchorEl={anchorEl}
      placement={placement}
      modifiers={[
        {
          name: 'eventListeners',
          options: { scroll: !disableScrollUpdate },
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
          if (
            anchorEl &&
            'contains' in anchorEl &&
            typeof (anchorEl as unknown as Node).contains === 'function' &&
            (anchorEl as unknown as Node).contains(e.target as Node)
          ) {
            return;
          }
          if (onClose) onClose(e, 'backdropClick');
        }}
      >
        <Paper
          elevation={2}
          className="fc-dropdown-container"
          onClick={(e) => e.stopPropagation()}
          onKeyDown={(e) => {
            e.stopPropagation();

            if (e.key === 'Escape') {
              if (onClose) onClose(e, 'escapeKeyDown');
            }

            if (e.key === 'Enter' && e.ctrlKey) {
              onApply();
            }

            if (enableSearchInput) {
              keyboardListNavigationHandler(e);
              switch (e.key) {
                case 'Delete':
                case 'Cancel':
                case 'Backspace':
                  if (document.activeElement === searchInput.current) {
                    return;
                  }
                  setSearchQuery('');
                  searchInput?.current?.focus();
              }
              // if the key is a single alphanumeric character without modifier
              if (/^[a-zA-Z0-9]\b/.test(e.key) && !hasAnyModifier(e)) {
                if (document.activeElement === searchInput.current) return;
                searchInput?.current?.focus();
                setSearchQuery((old) => old + e.key);
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
            width: 360,
          }}
        >
          {enableSearchInput && (
            <SearchInput
              searchInput={searchInput}
              searchQuery={searchQuery}
              fullWidth
              onChange={(e) => {
                setSearchQuery(e.currentTarget.value);
              }}
              onKeyDown={(e) => {
                if (e.key === 'ArrowDown' || (e.key === 'j' && e.ctrlKey)) {
                  const firstItem =
                    listContainerRef.current?.querySelector<HTMLElement>(
                      '[role="menuitem"], [role="option"], button',
                    );
                  firstItem?.focus();
                  e.preventDefault();
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
              paddingTop: '8px',
              overscrollBehavior: 'contain',
            }}
            tabIndex={-1}
            role="menu"
            key="options-menu-container"
            ref={listContainerRef}
            onWheel={(e) => e.stopPropagation()}
            onTouchMove={(e) => e.stopPropagation()}
          >
            {renderChild(searchQuery, (e) => {
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
              key="options-menu-footer"
            />
          )}
        </Paper>
      </ClickAwayListener>
    </Popper>
  );
}
