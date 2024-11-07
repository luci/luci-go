import { Checkbox, MenuItem, Menu, PopoverOrigin } from '@mui/material';

import { FilterOption, SelectedFilters } from './types';

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
}: {
  onClose?: (event: object, reason: 'backdropClick' | 'escapeKeyDown') => void;
  anchorEl: HTMLElement | null;
  open: boolean;
  option: FilterOption;
  selectedOptions: SelectedFilters;
  setSelectedOptions: React.Dispatch<React.SetStateAction<SelectedFilters>>;
  anchorOrigin?: PopoverOrigin | undefined;
}) {
  const flipOption = (o2Value: string) =>
    setSelectedOptions({
      ...selectedOptions,
      [option.value]: {
        ...(selectedOptions[option.value] ?? {}),
        [o2Value]: !selectedOptions[option.value]?.[o2Value],
      },
    });

  return (
    <Menu
      variant="selectedMenu"
      onClose={onClose}
      open={open}
      anchorEl={anchorEl}
      anchorOrigin={anchorOrigin}
    >
      {option.options.map((o2) => (
        <MenuItem
          key={`innerMenu-${option.value}-${o2.value}`}
          disableRipple
          onClick={() => flipOption(o2.value)}
        >
          <button
            style={{
              all: 'initial',
            }}
          >
            <Checkbox
              inputProps={{ 'aria-label': 'controlled' }}
              checked={!!selectedOptions[option.value]?.[o2.value]}
              tabIndex={-1}
            />
            {o2.label}
          </button>
        </MenuItem>
      ))}
      {/* Add apply/cancel buttons */}
    </Menu>
  );
}
