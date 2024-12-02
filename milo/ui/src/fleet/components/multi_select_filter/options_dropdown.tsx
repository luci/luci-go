import {
  Checkbox,
  MenuItem,
  Menu,
  PopoverOrigin,
  Button,
  Divider,
  MenuProps,
} from '@mui/material';
import { useState } from 'react';

import { HighlightCharacter } from '../highlight_character';

import { FilterOption, SelectedFilters } from './types';
import { keyboardUpDownHandler } from './utils';

export type OptionsDropdownProps = MenuProps & {
  onClose?: (event: object, reason: 'backdropClick' | 'escapeKeyDown') => void;
  anchorEl: HTMLElement | null;
  open: boolean;
  option: FilterOption;
  selectedOptions: SelectedFilters;
  setSelectedOptions: React.Dispatch<React.SetStateAction<SelectedFilters>>;
  anchorOrigin?: PopoverOrigin | undefined;
  matches?: Record<string, number[]>;
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
  matches,
  ...menuProps
}: OptionsDropdownProps) {
  const [tempSelectedOptions, setTempSelectedOptions] =
    useState(selectedOptions);

  const flipOption = (o2Value: string) => {
    const currentValues =
      tempSelectedOptions[option.nameSpace]?.[option.value] ?? [];

    const newValues = currentValues.includes(o2Value)
      ? currentValues.filter((v) => v !== o2Value)
      : currentValues.concat(o2Value);

    setTempSelectedOptions({
      ...(tempSelectedOptions ?? {}),
      [option.nameSpace]: {
        ...tempSelectedOptions[option.nameSpace],
        [option.value]: newValues,
      },
    });
  };

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
        if (e.key === 'Enter' && e.ctrlKey) {
          confirmTempOptions();
        }

        if (onKeyDown) onKeyDown(e);
      }}
      {...menuProps}
    >
      <div
        css={{
          maxHeight: 200,
          overflow: 'auto',
        }}
        tabIndex={-1}
      >
        {option.options.map((o2, idx) => (
          <MenuItem
            key={`innerMenu-${option.value}-${o2.value}`}
            disableRipple
            onClick={(e) => {
              if (e.type === 'keydown' || e.type === 'keyup') {
                const parsedE =
                  e as unknown as React.KeyboardEvent<HTMLLIElement>;
                if (parsedE.key === ' ') return;
              }
              flipOption(o2.value);
            }}
            onKeyDown={keyboardUpDownHandler}
            // eslint-disable-next-line jsx-a11y/no-autofocus
            autoFocus={idx === 0}
          >
            <button
              css={{
                all: 'initial',
                display: 'flex',
                alignItems: 'center',
              }}
            >
              <Checkbox
                checked={
                  !!tempSelectedOptions[option.nameSpace]?.[
                    option.value
                  ]?.includes(o2.value)
                }
                tabIndex={-1}
              />
              <HighlightCharacter
                variant="body2"
                highlights={matches?.[o2.value]}
              >
                {o2.label}
              </HighlightCharacter>
            </button>
          </MenuItem>
        ))}
      </div>
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
    </Menu>
  );
}
