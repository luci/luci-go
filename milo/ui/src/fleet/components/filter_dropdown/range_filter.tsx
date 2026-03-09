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

import { Slider, TextField } from '@mui/material';
import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
} from 'react';

export interface RangeFilterValue {
  min?: number;
  max?: number;
}

export interface RangeFilterProps {
  value: RangeFilterValue;
  onChange: (value: RangeFilterValue) => void;
  min?: number;
  max?: number;
}

export const RangeFilter = forwardRef(function RangeFilter(
  { value, onChange, min = 0, max = 10000 }: RangeFilterProps,
  ref,
) {
  const minInputRef = useRef<HTMLInputElement>(null);

  useImperativeHandle(ref, () => ({
    focus: () => {
      minInputRef.current?.focus();
    },
  }));

  // used by the input fields and may be outside of min/max temporarily
  const [textFieldValue, setTextFieldValue] = useState<
    [number | string | undefined, number | string | undefined]
  >([value.min, value.max]);

  // Update local state when value prop changes, but only if not editing?
  // Actually, standard pattern is to sync local state with props when props change.
  useEffect(() => {
    setTextFieldValue([value.min, value.max]);
  }, [value.min, value.max]);

  const isTextFieldValueValid = () => {
    const val0 =
      typeof textFieldValue[0] === 'string'
        ? parseInt(textFieldValue[0])
        : textFieldValue[0];
    const val1 =
      typeof textFieldValue[1] === 'string'
        ? parseInt(textFieldValue[1])
        : textFieldValue[1];

    if (
      val0 === undefined ||
      val1 === undefined ||
      isNaN(val0) ||
      isNaN(val1)
    ) {
      return true;
    }
    if (val0 < min || val1 > max) {
      return false;
    }
    return val0 <= val1;
  };

  const handleSliderChange = (_: Event, newValue: number | number[]) => {
    const val = newValue as [number, number];
    onChange({ min: val[0], max: val[1] });
    setTextFieldValue(val);
  };

  return (
    // eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions
    <div
      role="group"
      aria-label="Range filter"
      tabIndex={-1}
      css={{
        display: 'flex',
        flexDirection: 'column',
        gap: 20,
        width: 300,
        padding: '12px 20px',
        alignItems: 'center',
        outline: 'none',
      }}
      onKeyDown={(e) => e.stopPropagation()}
    >
      <div css={{ padding: '0 8px', width: '100%', boxSizing: 'border-box' }}>
        <Slider
          min={min}
          max={max}
          value={[value.min ?? min, value.max ?? max]}
          disableSwap
          onChange={handleSliderChange}
        />
      </div>
      <div
        css={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          gap: 8,
        }}
      >
        <TextField
          error={!isTextFieldValueValid()}
          value={textFieldValue[0] ?? ''}
          label="Min"
          type="number"
          size="small"
          inputRef={minInputRef}
          slotProps={{
            inputLabel: {
              shrink: true,
            },
          }}
          onChange={(e) => {
            const rawValue = e.target.value;
            const parsed = parseInt(rawValue);

            if (rawValue === '') {
              setTextFieldValue(['', textFieldValue[1]]);
              onChange({ min: undefined, max: value.max });
              return;
            }

            setTextFieldValue([rawValue, textFieldValue[1]]);

            // Apply immediately if valid and within bounds relative to other value
            if (
              !isNaN(parsed) &&
              parsed >= min &&
              parsed <= (value.max ?? max)
            ) {
              onChange({ min: parsed, max: value.max });
            }
          }}
        />
        <span>-</span>
        <TextField
          error={!isTextFieldValueValid()}
          value={textFieldValue[1] ?? ''}
          label="Max"
          type="number"
          size="small"
          slotProps={{
            inputLabel: {
              shrink: true,
            },
          }}
          onChange={(e) => {
            const rawValue = e.target.value;
            const parsed = parseInt(rawValue);

            if (rawValue === '') {
              setTextFieldValue([textFieldValue[0], '']);
              onChange({ min: value.min, max: undefined });
              return;
            }

            setTextFieldValue([textFieldValue[0], rawValue]);

            if (
              !isNaN(parsed) &&
              parsed <= max &&
              parsed >= (value.min ?? min)
            ) {
              onChange({ min: value.min, max: parsed });
            }
          }}
        />
      </div>
    </div>
  );
});
