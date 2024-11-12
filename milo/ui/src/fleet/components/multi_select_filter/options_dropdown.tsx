import {
  Checkbox,
  MenuItem,
  Menu,
  PopoverOrigin,
  Button,
  Divider,
  Typography,
  MenuProps,
} from '@mui/material';
import { useState } from 'react';

import { FilterOption, SelectedFilters } from './types';

export type OptionsDropdownProps = MenuProps & {
  onClose?: (event: object, reason: 'backdropClick' | 'escapeKeyDown') => void;
  anchorEl: HTMLElement | null;
  open: boolean;
  option: FilterOption;
  selectedOptions: SelectedFilters;
  setSelectedOptions: React.Dispatch<React.SetStateAction<SelectedFilters>>;
  anchorOrigin?: PopoverOrigin | undefined;
};

export function OptionsDropdown({
  onClose,
  anchorEl,
  open,
  option,
  selectedOptions,
  setSelectedOptions,
  anchorOrigin = {
    vertical: 'top',
    horizontal: 'right',
  },
  onKeyDown,
  ...menuProps
}: OptionsDropdownProps) {
  const [tempSelectedOptions, setTempSelectedOptions] =
    useState(selectedOptions);

  const flipOption = (o2Value: string) =>
    setTempSelectedOptions({
      ...tempSelectedOptions,
      [option.value]: {
        ...(tempSelectedOptions[option.value] ?? {}),
        [o2Value]: !tempSelectedOptions[option.value]?.[o2Value],
      },
    });

  const resetTempOptions = () => setTempSelectedOptions(selectedOptions);
  const confirmTempOptions = () => {
    setSelectedOptions(tempSelectedOptions);
    if (onClose) onClose({}, 'backdropClick');
  };

  return (
    <Menu
      variant="selectedMenu"
      onClose={(...args) => {
        if (onClose) onClose(...args);
        resetTempOptions();
      }}
      open={open}
      anchorEl={anchorEl}
      anchorOrigin={anchorOrigin}
      elevation={2}
      onKeyDown={(e: React.KeyboardEvent<HTMLDivElement>) => {
        if (e.key === 'Enter') confirmTempOptions();

        if (onKeyDown) onKeyDown(e);
      }}
      {...menuProps}
    >
      {option.options.map((o2) => (
        <MenuItem
          key={`innerMenu-${option.value}-${o2.value}`}
          disableRipple
          onClick={(e) => {
            if (e.type === 'keydown' || e.type === 'keyup') {
              const parsedE =
                e as unknown as React.KeyboardEvent<HTMLLIElement>;
              if (parsedE.key === 'Enter' || parsedE.key === ' ') return;
            }
            flipOption(o2.value);
          }}
          onKeyDown={(e) => {
            switch (e.key) {
              case 'ArrowDown':
                (e.currentTarget.nextSibling as HTMLElement)?.focus();
                break;
              case 'ArrowUp':
                (e.currentTarget.previousSibling as HTMLElement)?.focus();
                break;
              case ' ':
                e.currentTarget.click();
                break;
            }
          }}
        >
          <button
            css={{
              all: 'initial',
              display: 'flex',
              alignItems: 'center',
            }}
          >
            <Checkbox
              checked={!!tempSelectedOptions[option.value]?.[o2.value]}
            />
            <Typography variant="body2">{o2.label}</Typography>
          </button>
        </MenuItem>
      ))}
      <Divider />
      <div
        css={{
          display: 'flex',
          gap: 10,
          padding: 10,
          paddingLeft: 60,
        }}
      >
        <Button disableElevation onClick={resetTempOptions}>
          Cancel
        </Button>
        <Button
          disableElevation
          variant="contained"
          onClick={confirmTempOptions}
        >
          Confirm
        </Button>
      </div>
      {/* Add apply/cancel buttons */}
    </Menu>
  );
}
