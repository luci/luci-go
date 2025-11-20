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

import { Box, Button, TextField, Typography, Chip, Stack } from '@mui/material';
import { DateTimePicker } from '@mui/x-date-pickers/DateTimePicker';
import { DateTime } from 'luxon';
import { useReducer, useEffect } from 'react';
import { v4 as uuidv4 } from 'uuid';

import {
  MAXIMUM_PAGE_SIZE,
  SearchMeasurementsRequest,
  Timestamp,
} from '@/crystal_ball/hooks/use_android_perf_api';

/**
 * Helper to convert DateTime to Timestamp.
 * @param date - raw value from the DateTimePicker.
 * @returns date in Timestamp proto format.
 */
const dateToTimestamp = (date: DateTime | null): Timestamp | undefined => {
  if (!date) return undefined;
  const seconds = date.toSeconds();
  const nanos = 0;
  return { seconds, nanos };
};

/**
 * Helper to convert Timestamp to DateTime.
 * @param timestamp - raw Timestamp proto value.
 * @returns timestamp value converted to a DateTime value.
 */
const timestampToDate = (timestamp: Timestamp | undefined): DateTime | null => {
  if (!timestamp || timestamp.seconds === undefined) return null;
  return DateTime.fromSeconds(timestamp.seconds).toUTC();
};

/**
 * State structure for the form.
 */
interface FormState {
  testNameFilter: string;
  buildCreateStartTime: DateTime | null;
  buildCreateEndTime: DateTime | null;
  lastNDays?: number;
  buildBranch: string;
  buildTarget: string;
  atpTestNameFilter: string;
  metricKeys: string[];
  currentMetricKey: string;
  extraColumns: string[];
  currentExtraColumn: string;
}

enum Action {
  SET_TEXT_FIELD = 'SET_TEXT_FIELD',
  SET_N_DAYS = 'SET_N_DAYS',
  SET_START_TIME = 'SET_START_TIME',
  SET_END_TIME = 'SET_END_TIME',
  ADD_METRIC_KEY = 'ADD_METRIC_KEY',
  DELETE_METRIC_KEY = 'DELETE_METRIC_KEY',
  SET_CURRENT_METRIC_KEY = 'SET_CURRENT_METRIC_KEY',
  ADD_EXTRA_COLUMN = 'ADD_EXTRA_COLUMN',
  DELETE_EXTRA_COLUMN = 'DELETE_EXTRA_COLUMN',
  SET_CURRENT_EXTRA_COLUMN = 'SET_CURRENT_EXTRA_COLUMN',
  RESET_FORM = 'RESET_FORM',
}

/**
 * Actions for the reducer.
 */
type FormAction =
  | { type: Action.SET_TEXT_FIELD; field: keyof FormState; value: string }
  | { type: Action.SET_N_DAYS; value: string }
  | { type: Action.SET_START_TIME; value: DateTime | null }
  | { type: Action.SET_END_TIME; value: DateTime | null }
  | { type: Action.ADD_METRIC_KEY; value: string }
  | { type: Action.DELETE_METRIC_KEY; value: string }
  | { type: Action.SET_CURRENT_METRIC_KEY; value: string }
  | { type: Action.ADD_EXTRA_COLUMN; value: string }
  | { type: Action.DELETE_EXTRA_COLUMN; value: string }
  | { type: Action.SET_CURRENT_EXTRA_COLUMN; value: string }
  | { type: Action.RESET_FORM; payload: Partial<SearchMeasurementsRequest> };

/**
 * Reducer function to manage form state.
 */
function formReducer(state: FormState, action: FormAction): FormState {
  switch (action.type) {
    case Action.SET_TEXT_FIELD:
      return { ...state, [action.field]: action.value };
    case Action.SET_N_DAYS:
      if (!action.value) {
        return { ...state, lastNDays: undefined };
      }
      const numValue = parseInt(action.value, 10);
      return {
        ...state,
        lastNDays: isNaN(numValue) ? undefined : numValue, // Store undefined if NaN
      };
    case Action.SET_START_TIME:
      return { ...state, buildCreateStartTime: action.value };
    case Action.SET_END_TIME:
      return { ...state, buildCreateEndTime: action.value };
    case Action.ADD_METRIC_KEY: {
      const trimmedValue = action.value.trim();
      return trimmedValue && !state.metricKeys.includes(trimmedValue)
        ? {
            ...state,
            metricKeys: [...state.metricKeys, trimmedValue],
            currentMetricKey: '',
          }
        : { ...state, currentMetricKey: '' };
    }
    case Action.DELETE_METRIC_KEY:
      return {
        ...state,
        metricKeys: state.metricKeys.filter((key) => key !== action.value),
      };
    case Action.SET_CURRENT_METRIC_KEY:
      return { ...state, currentMetricKey: action.value };
    case Action.ADD_EXTRA_COLUMN: {
      const trimmedValue = action.value.trim();
      return trimmedValue && !state.extraColumns.includes(trimmedValue)
        ? {
            ...state,
            extraColumns: [...state.extraColumns, trimmedValue],
            currentExtraColumn: '',
          }
        : { ...state, currentExtraColumn: '' };
    }
    case Action.DELETE_EXTRA_COLUMN:
      return {
        ...state,
        extraColumns: state.extraColumns.filter((col) => col !== action.value),
      };
    case Action.SET_CURRENT_EXTRA_COLUMN:
      return { ...state, currentExtraColumn: action.value };
    case Action.RESET_FORM:
      return initializeState(action.payload);
    default:
      return state;
  }
}

/**
 * Function to initialize the state.
 */
const initializeState = (
  initialRequest: Partial<SearchMeasurementsRequest>,
): FormState => ({
  testNameFilter: initialRequest.testNameFilter || '',
  buildCreateStartTime: timestampToDate(initialRequest.buildCreateStartTime),
  buildCreateEndTime: timestampToDate(initialRequest.buildCreateEndTime),
  lastNDays: initialRequest.lastNDays,
  buildBranch: initialRequest.buildBranch || '',
  buildTarget: initialRequest.buildTarget || '',
  atpTestNameFilter: initialRequest.atpTestNameFilter || '',
  metricKeys: initialRequest.metricKeys || [],
  currentMetricKey: '',
  extraColumns: initialRequest.extraColumns || [],
  currentExtraColumn: '',
});

/**
 * Props for the Search Measurements form.
 */
interface SearchMeasurementsFormProps {
  /**
   * Callback to call with the completed SeearchMeasurementsRequest.
   * @param request - SearchMeasurementsRequest from the form.
   */
  onSubmit: (request: SearchMeasurementsRequest) => void;

  /**
   * Optional to flag to indicate an API call is being made.
   */
  isSubmitting?: boolean;

  /**
   * Optional initial request state to populate the form with.
   */
  initialRequest?: Partial<SearchMeasurementsRequest>;
}

export function SearchMeasurementsForm({
  onSubmit,
  isSubmitting,
  initialRequest = {},
}: SearchMeasurementsFormProps) {
  const [state, dispatch] = useReducer(
    formReducer,
    initialRequest,
    initializeState,
  );

  // Effect to reset form when initialRequest changes
  useEffect(() => {
    dispatch({ type: Action.RESET_FORM, payload: initialRequest });
  }, [initialRequest]);

  const handleSubmit = (event: React.FormEvent) => {
    event.preventDefault();
    const request: SearchMeasurementsRequest = {
      testNameFilter: state.testNameFilter || undefined,
      buildCreateStartTime: dateToTimestamp(state.buildCreateStartTime),
      buildCreateEndTime: dateToTimestamp(state.buildCreateEndTime),
      lastNDays: state.lastNDays,
      buildBranch: state.buildBranch || undefined,
      buildTarget: state.buildTarget || undefined,
      atpTestNameFilter: state.atpTestNameFilter || undefined,
      metricKeys: state.metricKeys.length > 0 ? state.metricKeys : undefined,
      extraColumns:
        state.extraColumns.length > 0 ? state.extraColumns : undefined,
      pageSize: MAXIMUM_PAGE_SIZE,
    };
    onSubmit(request);
  };

  return (
    <Box
      component="form"
      onSubmit={handleSubmit}
      noValidate
      sx={{ mt: 1, p: 2 }}
    >
      <Typography variant="h6" gutterBottom>
        Search Measurements
      </Typography>

      <TextField
        label="Test Name Filter"
        value={state.testNameFilter}
        onChange={(e) =>
          dispatch({
            type: Action.SET_TEXT_FIELD,
            field: 'testNameFilter',
            value: e.target.value,
          })
        }
        fullWidth
        margin="normal"
        variant="outlined"
        helperText='e.g., "ExampleGroup.ExampleSubGroup#ExampleTestName"'
      />

      <TextField
        label="ATP Test Name Filter"
        value={state.atpTestNameFilter}
        onChange={(e) =>
          dispatch({
            type: Action.SET_TEXT_FIELD,
            field: 'atpTestNameFilter',
            value: e.target.value,
          })
        }
        fullWidth
        margin="normal"
        variant="outlined"
        helperText='e.g., "v2/example-test-group/example-test-name"'
      />

      <TextField
        label="Build Branch"
        value={state.buildBranch}
        onChange={(e) =>
          dispatch({
            type: Action.SET_TEXT_FIELD,
            field: 'buildBranch',
            value: e.target.value,
          })
        }
        fullWidth
        margin="normal"
        variant="outlined"
        helperText='e.g., "example_git_main"'
      />

      <TextField
        label="Build Target"
        value={state.buildTarget}
        onChange={(e) =>
          dispatch({
            type: Action.SET_TEXT_FIELD,
            field: 'buildTarget',
            value: e.target.value,
          })
        }
        fullWidth
        margin="normal"
        variant="outlined"
        helperText='e.g., "example-build-target"'
      />

      <Typography variant="body2" color="textSecondary" sx={{ mt: 2 }}>
        Filter by Last N Days (overrides start/end times):
      </Typography>
      <TextField
        label="Last N Days"
        type="number"
        value={state.lastNDays === undefined ? '' : String(state.lastNDays)}
        onChange={(e) =>
          dispatch({
            type: Action.SET_N_DAYS,
            value: e.target.value,
          })
        }
        fullWidth
        margin="normal"
        variant="outlined"
      />

      <Typography variant="body2" color="textSecondary" sx={{ mt: 2 }}>
        OR Filter by Time Range:
      </Typography>
      <Stack direction="row" spacing={2} sx={{ mt: 1 }}>
        <DateTimePicker
          label="Build Create Start Time"
          value={state.buildCreateStartTime}
          onChange={(newValue) =>
            dispatch({ type: Action.SET_START_TIME, value: newValue })
          }
          disabled={!!state.lastNDays}
          slotProps={{ textField: { fullWidth: true, margin: 'normal' } }}
        />
        <DateTimePicker
          label="Build Create End Time"
          value={state.buildCreateEndTime}
          onChange={(newValue) =>
            dispatch({ type: Action.SET_END_TIME, value: newValue })
          }
          disabled={!!state.lastNDays}
          slotProps={{ textField: { fullWidth: true, margin: 'normal' } }}
        />
      </Stack>

      {/* Metric Keys Input */}
      <Box sx={{ mt: 2 }}>
        <TextField
          label="Add Metric Key"
          value={state.currentMetricKey}
          onChange={(e) =>
            dispatch({
              type: Action.SET_CURRENT_METRIC_KEY,
              value: e.target.value,
            })
          }
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              dispatch({
                type: Action.ADD_METRIC_KEY,
                value: state.currentMetricKey,
              });
            }
          }}
          fullWidth
          margin="normal"
          variant="outlined"
          helperText='Press Enter to add a key (e.g., "sample-metric-key-A")'
        />
        <Stack
          direction="row"
          spacing={1}
          useFlexGap
          flexWrap="wrap"
          sx={{ mb: 1 }}
        >
          {state.metricKeys.map((key) => (
            <Chip
              key={uuidv4()}
              label={key}
              onDelete={() =>
                dispatch({ type: Action.DELETE_METRIC_KEY, value: key })
              }
            />
          ))}
        </Stack>
      </Box>

      {/* Extra Columns Input */}
      <Box sx={{ mt: 2 }}>
        <TextField
          label="Add Extra Column"
          value={state.currentExtraColumn}
          onChange={(e) =>
            dispatch({
              type: Action.SET_CURRENT_EXTRA_COLUMN,
              value: e.target.value,
            })
          }
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              dispatch({
                type: Action.ADD_EXTRA_COLUMN,
                value: state.currentExtraColumn,
              });
            }
          }}
          fullWidth
          margin="normal"
          variant="outlined"
          helperText='Press Enter to add a column (e.g., "board")'
        />
        <Stack
          direction="row"
          spacing={1}
          useFlexGap
          flexWrap="wrap"
          sx={{ mb: 1 }}
        >
          {state.extraColumns.map((col) => (
            <Chip
              key={uuidv4()}
              label={col}
              onDelete={() =>
                dispatch({ type: Action.DELETE_EXTRA_COLUMN, value: col })
              }
            />
          ))}
        </Stack>
      </Box>

      <Box sx={{ mt: 3 }}>
        <Button
          type="submit"
          variant="contained"
          disabled={isSubmitting}
          color="primary"
        >
          {isSubmitting ? 'Searching...' : 'Search'}
        </Button>
      </Box>
    </Box>
  );
}
