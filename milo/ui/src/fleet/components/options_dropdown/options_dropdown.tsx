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

import { Menu, MenuOwnerState, MenuProps, PopoverOrigin } from '@mui/material';
import React, { ReactNode, useRef, useState } from 'react';

import { hasAnyModifier, keyboardListNavigationHandler } from '../../utils';
import { SearchInput } from '../search_input';

import { Footer } from './footer';

type OptionsDropdownProps = MenuProps & {
  onClose: () => void;
  onApply: () => void;
  renderChild: (
    searchQuery: string,
    onNavigateUp: (e: React.KeyboardEvent) => void,
  ) => ReactNode;
  anchorEl: HTMLElement | null;
  open: boolean;
  anchorOrigin?: PopoverOrigin | undefined;
  enableSearchInput?: boolean;
  maxHeight?: number;
  onResetClick?: React.MouseEventHandler<HTMLButtonElement>;
  footerButtons?: ('reset' | 'cancel' | 'apply')[];
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
  onKeyDown,
  enableSearchInput = false,
  maxHeight = 215,
  onResetClick,
  footerButtons = ['apply', 'cancel'],
  ...menuProps
}: OptionsDropdownProps) {
  const searchInput = useRef<HTMLInputElement>(null);
  const [searchQuery, setSearchQuery] = useState('');

  const slotProps = { ...(menuProps.slotProps ?? {}) };
  const listProps = (ownerState: MenuOwnerState) => {
    const incomingProps =
      typeof slotProps.list === 'function'
        ? slotProps.list(ownerState)
        : slotProps.list;

    return {
      ...incomingProps,

      sx: [
        {
          paddingTop: '8px',
          paddingBottom: 0,
        },
        incomingProps?.sx,
      ],
    };
  };
  const slotPropsMerged = { ...slotProps, ...{ list: listProps } };

  return (
    <Menu
      variant="selectedMenu"
      onClose={(...args) => {
        if (onClose) onClose(...args);
        if (enableSearchInput) {
          setSearchQuery('');
        }
      }}
      open={open}
      anchorEl={anchorEl}
      anchorOrigin={anchorOrigin}
      elevation={2}
      sx={{
        zIndex: 1401, // luci's cookie_consent_bar is 1400
      }}
      {...menuProps}
      slotProps={slotPropsMerged}
    >
      {/* Stop propagation to prevent the caller from handling the event inside the dropdown */}
      {/* eslint-disable-next-line jsx-a11y/no-static-element-interactions*/}
      <div
        onClick={(e) => e.stopPropagation()}
        onKeyDown={(e) => {
          e.stopPropagation();

          if (e.key === 'Escape') {
            onClose();
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
                setSearchQuery('');
                searchInput?.current?.focus();
            }
            // if the key is a single alphanumeric character without modifier
            if (/^[a-zA-Z0-9]\b/.test(e.key) && !hasAnyModifier(e)) {
              searchInput?.current?.focus();
              setSearchQuery((old) => old + e.key);
              e.preventDefault(); // Avoid race condition to type twice in the input
            }
          }

          if (onKeyDown) onKeyDown(e);
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
              keyboardListNavigationHandler(e, undefined, () => {
                onClose();
                e.preventDefault();
                e.stopPropagation();
              });
            }}
          />
        )}
        <div
          css={{
            maxHeight: maxHeight,
            overflow: 'hidden',
            maxWidth: 500,
          }}
          tabIndex={-1}
          key="options-menu-container"
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
              if (onClose) onClose(e, 'escapeKeyDown');
            }}
            onApplyClick={onApply}
            onResetClick={onResetClick}
            key="options-menu-footer"
          />
        )}
      </div>
    </Menu>
  );
}
