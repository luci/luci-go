// Copyright 2025 The LUCI Authors.
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

import { DatePicker } from '@mui/x-date-pickers';
import { DateTime } from 'luxon';
import { useEffect } from 'react';

import { OptionComponentProps } from '@/fleet/components/filter_dropdown/filter_dropdown_old';
import { fromLuxonDateTime, toLuxonDateTime } from '@/fleet/utils/dates';

import {
  DateFilterData,
  ResourceRequestInsightsOptionComponentProps,
} from './use_rri_filters';

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
              // if zIndex is not set it defaults to 1300, which is lower than the rest of the page and causes problems.
              // it happened in nested menus (specifically selected chip component), where this is rendered below another menu
              zIndex: 1500,
            },
          },
        }}
      />
    </div>
  );
};

export const DateFilter = ({
  optionComponentProps: { onFiltersChange, onClose, filters, option },
}: OptionComponentProps<ResourceRequestInsightsOptionComponentProps>) => {
  useEffect(() => () => onClose(), [onClose]);

  const dateFilterData = filters
    ? (filters[option.value] as DateFilterData | undefined)
    : undefined;

  const updateFilter = (newDate: DateFilterData | undefined): void => {
    if (!newDate || (!newDate.min && !newDate.max)) {
      onFiltersChange({
        ...filters,
        [option.value]: undefined,
      });
      return;
    }
    onFiltersChange({
      ...filters,
      [option.value]: newDate,
    });
  };

  return (
    <div
      css={{
        display: 'flex',
        gap: 8,
        width: 400,
        padding: '12px 8px',
      }}
    >
      <CustomDatePicker
        label={'From'}
        value={toLuxonDateTime(dateFilterData?.min) || null}
        onChange={(date) => {
          updateFilter({
            min: fromLuxonDateTime(date),
            max: dateFilterData?.max,
          });
        }}
      />
      <CustomDatePicker
        label={'To'}
        value={toLuxonDateTime(dateFilterData?.max) || null}
        onChange={(date) => {
          updateFilter({
            min: dateFilterData?.min,
            max: fromLuxonDateTime(date),
          });
        }}
      />
    </div>
  );
};
