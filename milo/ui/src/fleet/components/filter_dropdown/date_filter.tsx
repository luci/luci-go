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

import { DatePicker, LocalizationProvider } from '@mui/x-date-pickers';
import { DateTime } from 'luxon';
import { forwardRef, useImperativeHandle, useRef } from 'react';

import { SafeAdapterLuxon } from '@/fleet/adapters/date_adapter';
import { DateFilterValue } from '@/fleet/types';

export interface DateFilterProps {
  value: DateFilterValue;
  onChange: (value: DateFilterValue) => void;
}

interface CustomDatePickerProps {
  label: string;
  value: DateTime<true> | null;
  onChange: (date: DateTime<true> | null) => void;
  inputRef?: React.Ref<HTMLInputElement>;
}

const CustomDatePicker = ({
  label,
  value,
  onChange,
  inputRef,
}: CustomDatePickerProps) => {
  return (
    <div css={{ flex: 1 }}>
      <LocalizationProvider dateAdapter={SafeAdapterLuxon}>
        <DatePicker
          label={label}
          value={value}
          onChange={onChange}
          slotProps={{
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
            },
          }}
        />
      </LocalizationProvider>
    </div>
  );
};

export const DateFilter = forwardRef(function DateFilter(
  { value, onChange }: DateFilterProps,
  ref,
) {
  const firstInputRef = useRef<HTMLInputElement>(null);

  useImperativeHandle(ref, () => ({
    focus: () => {
      firstInputRef.current?.focus();
    },
  }));

  const updateFilter = (newDate: DateFilterValue): void => {
    if (!newDate.min && !newDate.max) {
      onChange({});
      return;
    }
    onChange(newDate);
  };

  const getValidDate = (date?: Date) => {
    if (!date) return null;
    const dateTime = DateTime.fromJSDate(date);
    return dateTime.isValid ? dateTime : null;
  };

  return (
    // eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions
    <div
      role="group"
      aria-label="Date filter"
      tabIndex={-1}
      css={{
        display: 'flex',
        gap: 8,
        width: 400,
        padding: '12px 8px',
        outline: 'none',
      }}
      onKeyDown={(e) => e.stopPropagation()}
    >
      <CustomDatePicker
        label={'From'}
        value={getValidDate(value.min)}
        onChange={(date) => {
          updateFilter({
            min: date?.toJSDate() || undefined,
            max: value.max,
          });
        }}
        inputRef={firstInputRef}
      />
      <CustomDatePicker
        label={'To'}
        value={getValidDate(value.max)}
        onChange={(date) => {
          updateFilter({
            min: value.min,
            max: date?.toJSDate() || undefined,
          });
        }}
      />
    </div>
  );
});
