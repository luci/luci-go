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
  Alert,
  Box,
  Button,
  Chip,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import React, { useEffect, useReducer, useState } from 'react';

import { MAXIMUM_PAGE_SIZE } from '@/crystal_ball/constants';
import { SearchMeasurementsRequest } from '@/crystal_ball/types';
import { ValidationErrors, validateSearchRequest } from '@/crystal_ball/utils';

/**
 * State structure for the form.
 */
interface FormState {
  testNameFilter: string;
  buildBranch: string;
  buildTarget: string;
  atpTestNameFilter: string;
  metricKeys: string[];
  currentMetricKey: string;
}

/**
 * Action identifier.
 */
enum Action {
  SET_TEXT_FIELD = 'SET_TEXT_FIELD',
  ADD_METRIC_KEY = 'ADD_METRIC_KEY',
  DELETE_METRIC_KEY = 'DELETE_METRIC_KEY',
  SET_CURRENT_METRIC_KEY = 'SET_CURRENT_METRIC_KEY',
  RESET_FORM = 'RESET_FORM',
}

/**
 * Actions for the reducer.
 */
type FormAction =
  | { type: Action.SET_TEXT_FIELD; field: keyof FormState; value: string }
  | { type: Action.ADD_METRIC_KEY; value: string }
  | { type: Action.DELETE_METRIC_KEY; value: string }
  | { type: Action.SET_CURRENT_METRIC_KEY; value: string }
  | { type: Action.RESET_FORM; payload: Partial<SearchMeasurementsRequest> };

/**
 * Reducer function to manage form state.
 */
function formReducer(state: FormState, action: FormAction): FormState {
  switch (action.type) {
    case Action.SET_TEXT_FIELD:
      return { ...state, [action.field]: action.value };
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
  buildBranch: initialRequest.buildBranch || '',
  buildTarget: initialRequest.buildTarget || '',
  atpTestNameFilter: initialRequest.atpTestNameFilter || '',
  metricKeys: initialRequest.metricKeys || [],
  currentMetricKey: '',
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

/**
 * Form to fill out a Search Measurements request.
 */
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
  const [errors, setErrors] = useState<ValidationErrors>({});
  const [showErrors, setShowErrors] = useState(false);

  useEffect(() => {
    dispatch({ type: Action.RESET_FORM, payload: initialRequest });

    if (Object.keys(initialRequest).length > 0) {
      const initialErrors = validateSearchRequest(initialRequest);
      setErrors(initialErrors);
      setShowErrors(Object.keys(initialErrors).length > 0);
    } else {
      setErrors({});
      setShowErrors(false);
    }
  }, [initialRequest]);

  const handleSubmit = (event: React.FormEvent) => {
    event.preventDefault();
    setShowErrors(true);
    const request: SearchMeasurementsRequest = {
      testNameFilter: state.testNameFilter || undefined,
      buildBranch: state.buildBranch || undefined,
      buildTarget: state.buildTarget || undefined,
      atpTestNameFilter: state.atpTestNameFilter || undefined,
      metricKeys: state.metricKeys,
      pageSize: MAXIMUM_PAGE_SIZE,
    };
    const currentErrors = validateSearchRequest(request);
    setErrors(currentErrors);

    if (Object.keys(currentErrors).length === 0) {
      onSubmit(request);
    }
  };

  return (
    <Box
      component="form"
      onSubmit={handleSubmit}
      noValidate
      sx={{ mt: 1, p: 2, border: '1px solid #e0e0e0', borderRadius: '4px' }}
    >
      <Typography variant="h6" gutterBottom sx={{ mb: 2 }}>
        Search Measurements
      </Typography>

      {showErrors && errors.metricKeys && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {errors.metricKeys}
        </Alert>
      )}

      <Stack spacing={2}>
        <Stack direction="row" spacing={2}>
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
            margin="dense"
            variant="outlined"
            size="small"
            helperText='e.g., "Group.SubGroup#TestName"'
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
            margin="dense"
            variant="outlined"
            size="small"
            helperText='e.g., "v2/group/test"'
          />
        </Stack>

        <Stack direction="row" spacing={2}>
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
            margin="dense"
            variant="outlined"
            size="small"
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
            margin="dense"
            variant="outlined"
            size="small"
            helperText='e.g., "example-target"'
          />
        </Stack>

        <Box>
          <TextField
            label="Add Metric Key *"
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
            margin="dense"
            variant="outlined"
            size="small"
            helperText='Press Enter to add a key (e.g., "sample-metric-key-A"). At least one is required.'
            error={showErrors && !!errors.metricKeys}
          />
          <Stack
            direction="row"
            spacing={1}
            useFlexGap
            flexWrap="wrap"
            sx={{ mt: 1 }}
          >
            {state.metricKeys.map((metricKey) => (
              <Chip
                key={metricKey}
                label={metricKey}
                onDelete={() =>
                  dispatch({ type: Action.DELETE_METRIC_KEY, value: metricKey })
                }
                size="small"
              />
            ))}
          </Stack>
        </Box>
      </Stack>

      <Box sx={{ mt: 3 }}>
        <Button
          type="submit"
          variant="contained"
          disabled={isSubmitting}
          color="primary"
          size="medium"
        >
          {isSubmitting ? 'Searching...' : 'Search'}
        </Button>
      </Box>
    </Box>
  );
}
