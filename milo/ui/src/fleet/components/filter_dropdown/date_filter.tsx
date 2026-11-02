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
import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
} from 'react';

import { SafeAdapterLuxon } from '@/fleet/adapters/date_adapter';
import { BLANK_VALUE } from '@/fleet/constants/filters';
import { colors } from '@/fleet/theme/colors';
import {
  datePickerPaperSx,
  datePickerTextFieldSx,
  datePresetChipSx,
} from '@/fleet/theme/recipes';
import { fontSizes } from '@/fleet/theme/tokens';
import { DateFilterValue } from '@/fleet/types';
import {
  getValidDateTime,
  inferActivePresetId,
  QUICK_RELATIVE_PRESETS,
  QuickRelativePreset,
} from '@/fleet/utils/dates';

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

const DATE_PICKER_POPPER_SLOT_PROPS = {
  placement: 'right-start' as const,
  modifiers: [
    {
      name: 'offset',
      options: {
        offset: [0, 24],
      },
    },
  ],
  sx: {
    zIndex: 1500,
  },
};

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
    popper: DATE_PICKER_POPPER_SLOT_PROPS,
    desktopPaper: {
      sx: datePickerPaperSx,
    },
    textField: {
      inputRef,
      size: 'small' as const,
      fullWidth: true,
      InputLabelProps: {
        shrink: true,
      },
      sx: datePickerTextFieldSx,
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

const FULL_DAY = 24 * 60 * 60 * 1000;

export const DateFilter = forwardRef(function DateFilter(
  { value, onChange, isFutureDisabled, isDateOnly = false }: DateFilterProps,
  ref: React.Ref<OptionComponentHandle>,
) {
  const firstInputRef = useRef<HTMLInputElement>(null);
  const [openId, setOpenId] = useState<'min' | 'max' | null>(null);
  const minTime = value.min?.getTime();
  const maxTime = value.max?.getTime();
  const [activePresetId, setActivePresetId] = useState<string | null>(() =>
    inferActivePresetId({ min: value.min, max: value.max }),
  );
  const lastInternalBoundsRef = useRef<{
    minTime?: number;
    maxTime?: number;
  }>({ minTime, maxTime });

  useEffect(() => {
    if (
      minTime === lastInternalBoundsRef.current.minTime &&
      maxTime === lastInternalBoundsRef.current.maxTime
    ) {
      return;
    }
    lastInternalBoundsRef.current = { minTime, maxTime };
    setActivePresetId(inferActivePresetId({ min: value.min, max: value.max }));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [minTime, maxTime]);

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

  const minDateValue = useMemo(
    () => getValidDateTime(value.min, isDateOnly),
    [value.min, isDateOnly],
  );
  const maxDateValue = useMemo(
    () => getValidDateTime(value.max, isDateOnly),
    [value.max, isDateOnly],
  );
  const toFilterDate = (date: DateTime | null): Date | undefined => {
    if (!date || !date.isValid) return undefined;
    return isDateOnly
      ? date.setZone('utc', { keepLocalTime: true }).startOf('day').toJSDate()
      : date.toJSDate();
  };
  const maxAllowedDate = useMemo(
    () =>
      isFutureDisabled
        ? isDateOnly
          ? DateTime.now().plus(FULL_DAY)
          : DateTime.now()
        : undefined,
    [isFutureDisabled, isDateOnly],
  );

  const handlePresetClick = (preset: QuickRelativePreset) => {
    if (activePresetId === preset.id) {
      lastInternalBoundsRef.current = {
        minTime: undefined,
        maxTime: undefined,
      };
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
    const nextMin = preset.bound === 'min' ? cutoff : undefined;
    const nextMax = preset.bound === 'max' ? cutoff : undefined;
    lastInternalBoundsRef.current = {
      minTime: nextMin?.getTime(),
      maxTime: nextMax?.getTime(),
    };
    setActivePresetId(preset.id);
    updateFilter({
      ...value,
      min: nextMin,
      max: nextMax,
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
              flexWrap: 'wrap',
              gap: '4px',
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
                  aria-pressed={isSelected}
                  onClick={() => handlePresetClick(preset)}
                  sx={datePresetChipSx}
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
          value={minDateValue}
          onChange={(date) => {
            const nextMin = toFilterDate(date);
            lastInternalBoundsRef.current = {
              minTime: nextMin?.getTime(),
              maxTime,
            };
            setActivePresetId(null);
            updateFilter({
              ...value,
              min: nextMin,
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
          value={maxDateValue}
          onChange={(date) => {
            const nextMax = toFilterDate(date);
            lastInternalBoundsRef.current = {
              minTime,
              maxTime: nextMax?.getTime(),
            };
            setActivePresetId(null);
            updateFilter({
              ...value,
              max: nextMax,
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
      <Box role="group" aria-label="Blank options" sx={{ padding: '6px 0' }}>
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
              lastInternalBoundsRef.current = {
                minTime: undefined,
                maxTime: undefined,
              };
              setActivePresetId(null);
              updateFilter({
                onlyEmpty: true,
              });
            }}
            sx={{
              minWidth: 'unset',
              padding: '2px 6px',
              textTransform: 'none',
              fontSize: fontSizes.caption,
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
        </MenuItem>
      </Box>
    </Box>
  );
});
