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

import { Box } from '@mui/material';
import { DatePicker } from '@mui/x-date-pickers';
import { DateTime } from 'luxon';
import { useEffect, useRef } from 'react';

export interface DateFilterValue {
  min?: string; // ISO string
  max?: string; // ISO string
}

export interface DateFilterProps {
  value: DateFilterValue;
  onChange: (value: DateFilterValue) => void;
}

interface CustomDatePickerProps {
  label: string;
  value: DateTime<true> | null;
  onChange: (date: DateTime<true> | null) => void;
}

const CustomDatePicker = ({
  label,
  value,
  onChange,
}: CustomDatePickerProps) => {
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (label === 'From') {
      inputRef.current?.focus();
    }
  }, [label]);

  return (
    <div css={{ flex: 1 }}>
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
    </div>
  );
};

export function DateFilter({ value, onChange }: DateFilterProps) {
  const updateFilter = (newDate: DateFilterValue): void => {
    if (!newDate.min && !newDate.max) {
      onChange({});
      return;
    }
    onChange(newDate);
  };

  return (
    <Box
      sx={{
        display: 'flex',
        gap: '8px',
        width: 400,
        padding: '12px 8px',
      }}
      onKeyDown={(e) => e.stopPropagation()}
    >
      <CustomDatePicker
        label={'From'}
        value={value.min ? DateTime.fromISO(value.min) : null}
        onChange={(date) => {
          updateFilter({
            min: date?.toISODate() || undefined,
            max: value.max,
          });
        }}
      />
      <CustomDatePicker
        label={'To'}
        value={value.max ? DateTime.fromISO(value.max) : null}
        onChange={(date) => {
          updateFilter({
            min: value.min,
            max: date?.toISODate() || undefined,
          });
        }}
      />
    </Box>
  );
}
