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

import { Menu, MenuProps, PopoverOrigin } from '@mui/material';
import React, { ReactNode, useRef, useState } from 'react';

import { hasAnyModifier, keyboardUpDownHandler } from '../../utils';
import { SearchInput } from '../search_input';

import { Footer } from './footer';

type OptionsDropdownProps = MenuProps & {
  onClose?: (event: object, reason: 'backdropClick' | 'escapeKeyDown') => void;
  onApply: () => void;
  renderChild: (searchQuery: string) => ReactNode;
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
  maxHeight = 200,
  onResetClick,
  footerButtons = ['apply', 'cancel'],
  ...menuProps
}: OptionsDropdownProps) {
  const searchInput = useRef<HTMLInputElement>(null);
  const [searchQuery, setSearchQuery] = useState('');

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
      onKeyDown={(e: React.KeyboardEvent<HTMLDivElement>) => {
        if (e.key === 'Enter' && e.ctrlKey) {
          onApply();
        }

        if (enableSearchInput) {
          keyboardUpDownHandler(e);
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
      sx={{
        zIndex: 1401, // luci's cookie_consent_bar is 1400
      }}
      MenuListProps={{
        sx: {
          paddingTop: '8px',
          paddingBottom: 0,
        },
      }}
      {...menuProps}
    >
      <div>
        {enableSearchInput && (
          <div css={{ flexGrow: 1 }}>
            <SearchInput
              searchInput={searchInput}
              searchQuery={searchQuery}
              onChange={(e) => {
                setSearchQuery(e.currentTarget.value);
              }}
            />
          </div>
        )}
        <div
          css={{
            maxHeight: maxHeight,
            overflow: 'hidden',
          }}
          tabIndex={-1}
          key="options-menu-container"
        >
          {renderChild(searchQuery)}
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
