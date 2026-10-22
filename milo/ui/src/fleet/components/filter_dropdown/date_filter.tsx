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
  Box,
  Button,
  Checkbox,
  Chip,
  Divider,
  MenuItem,
  Typography,
} from '@mui/material';
import {
  DesktopDatePicker,
  DesktopDateTimePicker,
  LocalizationProvider,
} from '@mui/x-date-pickers';
import { DateTime } from 'luxon';
import { forwardRef, useImperativeHandle, useRef, useState } from 'react';

import { SafeAdapterLuxon } from '@/fleet/adapters/date_adapter';
import { BLANK_VALUE } from '@/fleet/constants/filters';
import { colors } from '@/fleet/theme/colors';
import { fleetColors, radii } from '@/fleet/theme/tokens';
import { DateFilterValue } from '@/fleet/types';

import { OptionComponentHandle } from './filter_dropdown';

export interface DateFilterProps {
  value: DateFilterValue;
  onChange: (value: DateFilterValue) => void;
  isFutureDisabled?: boolean;
  isDateOnly?: boolean;
}

interface CustomDateInputProps {
  label: string;
  value: DateTime<true> | null;
  onChange: (date: DateTime<true> | null) => void;
  inputRef?: React.Ref<HTMLInputElement>;
  open?: boolean;
  onOpen?: () => void;
  onClose?: () => void;
  maxDate?: DateTime<true>;
  isDateOnly?: boolean;
}

const CustomDateInput = ({
  label,
  value,
  onChange,
  inputRef,
  open,
  onOpen,
  onClose,
  maxDate,
  isDateOnly,
}: CustomDateInputProps) => {
  const sharedSlotProps = {
    field: {
      clearable: true,
    },
    popper: {
      sx: {
        zIndex: 1500,
      },
    },
    textField: {
      inputRef,
      size: 'small' as const,
      fullWidth: true,
      InputLabelProps: {
        shrink: true,
      },
      sx: {
        width: '100%',
        '& .MuiOutlinedInput-root, & .MuiPickersOutlinedInput-root': {
          borderRadius: radii.sm,
          backgroundColor: fleetColors.surface,
          '& fieldset': {
            borderColor: colors.grey[300],
          },
          '&:hover fieldset': {
            borderColor: colors.grey[500],
          },
          '&.Mui-focused fieldset': {
            borderColor: colors.blue[600],
          },
        },
        '& .MuiInputLabel-root': {
          color: colors.grey[700],
          fontWeight: 500,
          '&.Mui-focused': {
            color: colors.blue[600],
          },
        },
      },
    },
  };

  return (
    <Box sx={{ width: '100%' }}>
      <LocalizationProvider dateAdapter={SafeAdapterLuxon}>
        {isDateOnly ? (
          <DesktopDatePicker
            label={label}
            value={value}
            onChange={onChange}
            open={open}
            onOpen={onOpen}
            onClose={onClose}
            maxDate={maxDate}
            slotProps={sharedSlotProps}
            sx={{ width: '100%' }}
          />
        ) : (
          <DesktopDateTimePicker
            label={label}
            value={value}
            onChange={onChange}
            open={open}
            onOpen={onOpen}
            onClose={onClose}
            maxDateTime={maxDate}
            slotProps={sharedSlotProps}
            sx={{ width: '100%' }}
          />
        )}
      </LocalizationProvider>
    </Box>
  );
};

interface QuickRelativePreset {
  id: string;
  label: string;
  bound: 'min' | 'max';
  minutesAgo: number;
}

const QUICK_RELATIVE_PRESETS: QuickRelativePreset[] = [
  { id: 'last_15m', label: 'Last 15m', bound: 'min', minutesAgo: 15 },
  { id: 'last_1h', label: 'Last 1h', bound: 'min', minutesAgo: 60 },
  { id: 'older_30m', label: '> 30m ago', bound: 'max', minutesAgo: 30 },
  { id: 'older_1h', label: '> 1h ago', bound: 'max', minutesAgo: 60 },
  { id: 'older_24h', label: '> 24h ago', bound: 'max', minutesAgo: 24 * 60 },
];

export const DateFilter = forwardRef(function DateFilter(
  { value, onChange, isFutureDisabled, isDateOnly = false }: DateFilterProps,
  ref: React.Ref<OptionComponentHandle>,
) {
  const firstInputRef = useRef<HTMLInputElement>(null);
  const [openId, setOpenId] = useState<'min' | 'max' | null>(null);
  const [activePresetId, setActivePresetId] = useState<string | null>(null);

  useImperativeHandle(ref, () => ({
    focus: () => {
      firstInputRef.current?.focus();
    },
  }));

  const updateFilter = (newDate: DateFilterValue): void => {
    if (
      !newDate.min &&
      !newDate.max &&
      !newDate.onlyEmpty &&
      !newDate.excludeEmpty
    ) {
      onChange({});
      return;
    }
    onChange(newDate);
  };

  const getValidDate = (date?: Date) => {
    if (!date || isNaN(date.getTime())) return null;
    const dateTime = DateTime.fromJSDate(date);
    return dateTime.isValid ? dateTime : null;
  };
  const FULL_DAY = 24 * 60 * 60 * 1000;
  const maxAllowedDate = isFutureDisabled
    ? isDateOnly
      ? DateTime.now().plus(FULL_DAY)
      : DateTime.now()
    : undefined;

  const handlePresetClick = (preset: QuickRelativePreset) => {
    if (activePresetId === preset.id) {
      setActivePresetId(null);
      updateFilter({
        ...value,
        min: undefined,
        max: undefined,
      });
      return;
    }
    const cutoff = DateTime.now()
      .minus({ minutes: preset.minutesAgo })
      .startOf('minute')
      .toJSDate();
    setActivePresetId(preset.id);
    updateFilter({
      ...value,
      min: preset.bound === 'min' ? cutoff : undefined,
      max: preset.bound === 'max' ? cutoff : undefined,
    });
  };

  const presenceItemSx = {
    display: 'flex',
    alignItems: 'center',
    width: '100%',
    minHeight: 40,
    padding: '8px 16px',
    '&:hover .only-button, &:focus .only-button, &.Mui-focusVisible .only-button':
      {
        opacity: 1,
        visibility: 'visible',
      },
    '& .only-button': {
      opacity: 0,
      visibility: 'hidden',
      transition: 'opacity 0.2s, visibility 0.2s',
      marginLeft: 'auto',
    },
  };

  return (
    <Box
      role="group"
      aria-label="Date filter"
      tabIndex={-1}
      sx={{
        display: 'flex',
        flexDirection: 'column',
        width: 372,
        boxSizing: 'border-box',
        outline: 'none',
      }}
      onKeyDown={(e) => e.stopPropagation()}
    >
      {!isDateOnly && (
        <Box
          sx={{
            display: 'flex',
            flexDirection: 'column',
            gap: '8px',
            padding: '10px 16px 4px 16px',
          }}
        >
          <Typography
            variant="caption"
            sx={{
              color: colors.grey[700],
              fontWeight: 500,
              letterSpacing: '0.01em',
            }}
          >
            Quick presets
          </Typography>
          <Box
            sx={{
              display: 'flex',
              flexWrap: 'nowrap',
              gap: '6px',
            }}
          >
            {QUICK_RELATIVE_PRESETS.map((preset) => {
              const isSelected = activePresetId === preset.id;
              return (
                <Chip
                  key={preset.id}
                  label={preset.label}
                  size="small"
                  variant="outlined"
                  onClick={() => handlePresetClick(preset)}
                  sx={{
                    flex: '1 1 auto',
                    height: 28,
                    borderRadius: radii.sm,
                    fontSize: '0.75rem',
                    padding: 0,
                    '& .MuiChip-label, & .MuiChip-labelSmall': {
                      paddingLeft: '6px',
                      paddingRight: '6px',
                    },
                    borderColor: isSelected
                      ? colors.blue[600]
                      : colors.grey[300],
                    backgroundColor: isSelected
                      ? colors.blue[50]
                      : 'transparent',
                    color: isSelected ? colors.blue[600] : colors.grey[800],
                    fontWeight: isSelected ? 500 : 400,
                    transition: 'all 0.15s ease',
                    '&:hover': {
                      backgroundColor: colors.blue[50],
                      borderColor: isSelected
                        ? colors.blue[600]
                        : colors.blue[400],
                    },
                  }}
                />
              );
            })}
          </Box>
        </Box>
      )}
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
          gap: '20px',
          padding: '16px 16px 18px 16px',
          '& .MuiFormControl-root, & .MuiPickersTextField-root, & .MuiInputBase-root, & .MuiPickersInputBase-root':
            {
              width: '100%',
            },
        }}
      >
        <CustomDateInput
          label="From"
          value={getValidDate(value.min)}
          onChange={(date) => {
            setActivePresetId(null);
            updateFilter({
              ...value,
              min: date?.toJSDate() || undefined,
            });
          }}
          inputRef={firstInputRef}
          open={openId === 'min'}
          maxDate={maxAllowedDate}
          onOpen={() => setOpenId('min')}
          onClose={() => setOpenId(null)}
          isDateOnly={isDateOnly}
        />
        <CustomDateInput
          label="To"
          value={getValidDate(value.max)}
          onChange={(date) => {
            setActivePresetId(null);
            updateFilter({
              ...value,
              max: date?.toJSDate() || undefined,
            });
          }}
          open={openId === 'max'}
          maxDate={maxAllowedDate}
          onOpen={() => setOpenId('max')}
          onClose={() => setOpenId(null)}
          isDateOnly={isDateOnly}
        />
      </Box>
      <Divider sx={{ borderColor: colors.grey[200] }} />
      <Box sx={{ padding: '6px 0' }}>
        <MenuItem
          disableRipple
          onClick={() => {
            const nextOnlyEmpty = !value.onlyEmpty;
            updateFilter({
              ...value,
              onlyEmpty: nextOnlyEmpty || undefined,
              excludeEmpty: undefined,
            });
          }}
          sx={presenceItemSx}
        >
          <Checkbox
            size="small"
            checked={Boolean(value.onlyEmpty)}
            tabIndex={-1}
            slotProps={{
              input: {
                'aria-label': `Include ${BLANK_VALUE}`,
              },
            }}
            sx={{
              padding: 0,
              marginRight: '13px',
            }}
          />
          <Typography variant="body2" sx={{ flexGrow: 1 }}>
            Include{' '}
            <Box
              component="span"
              sx={{ fontStyle: 'italic', color: colors.grey[700] }}
            >
              {BLANK_VALUE}
            </Box>
          </Typography>
          <Button
            className="only-button"
            size="small"
            variant="text"
            color="primary"
            onClick={(e) => {
              e.stopPropagation();
              setActivePresetId(null);
              updateFilter({
                onlyEmpty: true,
              });
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
        </MenuItem>
        <MenuItem
          disableRipple
          onClick={() => {
            const nextExcludeEmpty = !value.excludeEmpty;
            updateFilter({
              ...value,
              excludeEmpty: nextExcludeEmpty || undefined,
              onlyEmpty: undefined,
            });
          }}
          sx={presenceItemSx}
        >
          <Checkbox
            size="small"
            checked={Boolean(value.excludeEmpty)}
            tabIndex={-1}
            slotProps={{
              input: {
                'aria-label': `Exclude ${BLANK_VALUE}`,
              },
            }}
            sx={{
              padding: 0,
              marginRight: '13px',
            }}
          />
          <Typography variant="body2" sx={{ flexGrow: 1 }}>
            Exclude{' '}
            <Box
              component="span"
              sx={{ fontStyle: 'italic', color: colors.grey[700] }}
            >
              {BLANK_VALUE}
            </Box>
          </Typography>
          <Button
            className="only-button"
            size="small"
            variant="text"
            color="primary"
            onClick={(e) => {
              e.stopPropagation();
              setActivePresetId(null);
              updateFilter({
                excludeEmpty: true,
              });
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
        </MenuItem>
      </Box>
    </Box>
  );
});
