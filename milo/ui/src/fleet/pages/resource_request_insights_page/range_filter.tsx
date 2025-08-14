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

import styled from '@emotion/styled';
import { Alert, CircularProgress, Slider, TextField } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useEffect, useState } from 'react';

import { OptionComponentProps } from '@/fleet/components/filter_dropdown/filter_dropdown_old';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';

import {
  RangeFilterData,
  ResourceRequestInsightsOptionComponentProps,
} from './use_rri_filters';

const DEFAULT_MIN_VALUE = 0;
const DEFAULT_MAX_VALUE = 10000;

const Container = styled.div`
  display: flex;
  justify-content: center;
  align-items: center;
  height: 100%;
  width: 100%;
  padding: 10px;
  box-sizing: border-box;
`;

/**
 * Currently only supports accepted quantity
 */
export const RangeFilter = ({
  optionComponentProps: { onFiltersChange, onClose, filters, option },
}: OptionComponentProps<ResourceRequestInsightsOptionComponentProps>) => {
  useEffect(() => () => onClose(), [onClose]);

  const client = useFleetConsoleClient();
  const query = useQuery(
    client.GetResourceRequestsMultiselectFilterValues.query({}),
  );

  const possibleValues = query.data?.acceptedQuantity;
  const [min, max] = possibleValues
    ? [Math.min(...possibleValues), Math.max(...possibleValues)]
    : [DEFAULT_MIN_VALUE, DEFAULT_MAX_VALUE];

  const getInitialValues = (): [number, number] => {
    if (!filters) {
      return [min, max];
    }

    const rangeFilterData = filters[option.value] as
      | RangeFilterData
      | undefined;
    if (!rangeFilterData) {
      return [min, max];
    }
    return [rangeFilterData.min ?? min, rangeFilterData.max ?? max];
  };

  // valid state used by the slider and for applying the filter
  const filterValue = filters
    ? (filters[option.value] as RangeFilterData | undefined)
    : { min: undefined, max: undefined };

  // used by the input fields and may be outside of min/max temporarily
  const [textFieldValue, setTextFieldValue] =
    useState<[number, number]>(getInitialValues());

  // whenever min/max value changes and textFieldValue is outside of these we want to clamp them between min/max
  useEffect(() => {
    setTextFieldValue((current) => [
      Math.max(current[0], min),
      Math.min(current[1], max),
    ]);
  }, [min, max]);

  const onChange = (newValues: RangeFilterData) => {
    onFiltersChange({
      ...filters,
      [option.value]: newValues,
    });
  };

  const isTextFieldValueValid = () => {
    if (!textFieldValue[0] || !textFieldValue[1]) {
      return true;
    }
    if (textFieldValue[0] < 0 || textFieldValue[1] > max) {
      return false;
    }
    return textFieldValue[0] <= textFieldValue[1];
  };

  if (query.isLoading) {
    return (
      <Container>
        <CircularProgress />
      </Container>
    );
  }

  if (query.isError) {
    return (
      <Container>
        <Alert css={{ width: '100%' }} severity="error">
          {query.error.message}
        </Alert>
      </Container>
    );
  }

  if (!query.data) {
    return (
      <Container>
        <Alert css={{ width: '100%' }} severity="error">
          Unexpected error
        </Alert>
      </Container>
    );
  }

  return (
    // disabling eslint warning - we want to clear the default behavior of targeting the search bar when typing in the inputs
    // eslint-disable-next-line jsx-a11y/no-static-element-interactions
    <div
      css={{
        display: 'flex',
        flexDirection: 'column',
        gap: 20,
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
          value={[filterValue?.min ?? min, filterValue?.max ?? max]}
          disableSwap
          onChange={(_, v) => {
            const newVal = v as [number, number];
            onChange({ min: newVal[0], max: newVal[1] });
            setTextFieldValue(newVal);
          }}
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
          slotProps={{
            inputLabel: {
              shrink: true,
            },
          }}
          onChange={(e) => {
            let parsed: number | undefined = parseInt(e.target.value);
            if (isNaN(parsed)) {
              parsed = min;
            }
            setTextFieldValue([parsed, textFieldValue[1]]);
            if (
              filterValue?.max === undefined ||
              parsed === undefined ||
              filterValue.max >= parsed
            ) {
              onChange({
                min: parsed,
                max: filterValue?.max,
              });
            }
          }}
        />
        <span>-</span>
        <TextField
          error={!isTextFieldValueValid()}
          value={textFieldValue[1]}
          label="Max"
          slotProps={{
            inputLabel: {
              shrink: true,
            },
          }}
          type="number"
          onChange={(e) => {
            let parsed: number | undefined = parseInt(e.target.value);
            if (isNaN(parsed)) {
              parsed = max;
            }
            setTextFieldValue([textFieldValue[0], parsed]);
            if (
              filterValue?.min === undefined ||
              parsed === undefined ||
              filterValue.min <= parsed
            ) {
              onChange({
                min: filterValue?.min,
                max: parsed,
              });
            }
          }}
        />
      </div>
    </div>
  );
};
