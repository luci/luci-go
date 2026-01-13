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

import { Box, Slider, TextField } from '@mui/material';
import { useEffect, useState } from 'react';

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

export function RangeFilter({
  value,
  onChange,
  min = 0,
  max = 10000,
}: RangeFilterProps) {
  // used by the input fields and may be outside of min/max temporarily
  const [textFieldValue, setTextFieldValue] = useState<[number, number]>([
    value.min ?? min,
    value.max ?? max,
  ]);

  // Update local state when value prop changes, but only if not editing?
  // Actually, standard pattern is to sync local state with props when props change.
  useEffect(() => {
    setTextFieldValue([value.min ?? min, value.max ?? max]);
  }, [value.min, value.max, min, max]);

  const isTextFieldValueValid = () => {
    if (
      textFieldValue[0] === undefined ||
      textFieldValue[1] === undefined ||
      isNaN(textFieldValue[0]) ||
      isNaN(textFieldValue[1])
    ) {
      return true;
    }
    if (textFieldValue[0] < min || textFieldValue[1] > max) {
      return false;
    }
    return textFieldValue[0] <= textFieldValue[1];
  };

  const handleSliderChange = (_: Event, newValue: number | number[]) => {
    const val = newValue as [number, number];
    onChange({ min: val[0], max: val[1] });
    setTextFieldValue(val);
  };

  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        gap: '20px',
        width: 300,
        padding: '12px 20px',
        alignItems: 'center',
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
          value={textFieldValue[0]}
          label="Min"
          type="number"
          size="small"
          slotProps={{
            inputLabel: {
              shrink: true,
            },
          }}
          onChange={(e) => {
            const parsed = parseInt(e.target.value);
            const newVal = isNaN(parsed) ? min : parsed;
            setTextFieldValue([newVal, textFieldValue[1]]);

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
          value={textFieldValue[1]}
          label="Max"
          type="number"
          size="small"
          slotProps={{
            inputLabel: {
              shrink: true,
            },
          }}
          onChange={(e) => {
            const parsed = parseInt(e.target.value);
            const newVal = isNaN(parsed) ? max : parsed;
            setTextFieldValue([textFieldValue[0], newVal]);

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
    </Box>
  );
}
