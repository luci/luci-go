// Copyright 2026 The LUCI Authors.
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

import { AutoAwesome } from '@mui/icons-material';
import {
  Alert,
  Autocomplete,
  Button,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  TextField,
} from '@mui/material';
import { useState } from 'react';
import { useDebounce } from 'react-use';

import {
  AUTOCOMPLETE_DEBOUNCE_DELAY_MS,
  MAX_SUGGEST_RESULTS,
  DATA_SPEC_ID,
} from '@/crystal_ball/constants';
import { useSuggestMeasurementFilterValues } from '@/crystal_ball/hooks';

/**
 * Props for {@link GenerateDashboardDialog}.
 */
export interface GenerateDashboardDialogProps {
  /** Whether the modal is open. */
  open: boolean;
  /** Callback for when the modal is closed. */
  onClose: () => void;
  /** A single, unified submit handler */
  onSubmit: (data: { prompt: string; metricKeys: string[] }) => Promise<void>;
  /** Let the parent tell the dialog if it's currently saving/generating */
  isPending?: boolean;
  /** Let the parent handle/pass down the error */
  errorMsg?: string;
}

/**
 * A dialog component for generating a new dashboard via LLM.
 */
export function GenerateDashboardDialog({
  open,
  onClose,
  onSubmit,
  isPending = false,
  errorMsg = '',
}: GenerateDashboardDialogProps) {
  const [prompt, setPrompt] = useState('');
  const [selectedMetrics, setSelectedMetrics] = useState<string[]>([]);
  const [inputValue, setInputValue] = useState('');
  const [debouncedQuery, setDebouncedQuery] = useState('');
  const [isFocused, setIsFocused] = useState(false);

  useDebounce(
    () => {
      setDebouncedQuery(inputValue);
    },
    AUTOCOMPLETE_DEBOUNCE_DELAY_MS,
    [inputValue],
  );

  // We use the default data spec placeholder because the dashboard hasn't been saved yet.
  const parent = `dashboardStates/-/dataSpecs/${DATA_SPEC_ID}`;

  // Suggest metric keys based on user input
  const { data: suggestionData, isLoading } = useSuggestMeasurementFilterValues(
    {
      parent,
      column: 'metric_key',
      query: debouncedQuery,
      maxResultCount: MAX_SUGGEST_RESULTS,
      filter: '',
    },
    {
      enabled: open && debouncedQuery.length > 0 && isFocused,
      retry: false,
    },
  );

  const options = suggestionData?.values ?? [];

  const handleSubmit = async () => {
    await onSubmit({ prompt, metricKeys: selectedMetrics });
  };

  const isFormValid = prompt.trim().length > 0;

  // Reset state when closed
  const handleClose = () => {
    if (!isPending) {
      setPrompt('');
      setSelectedMetrics([]);
      setInputValue('');
      onClose();
    }
  };

  return (
    <Dialog open={open} onClose={handleClose} fullWidth maxWidth="sm">
      <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
        <AutoAwesome color="primary" />
        Generate Dashboard
      </DialogTitle>
      <DialogContent>
        {errorMsg && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {errorMsg}
          </Alert>
        )}
        <TextField
          margin="dense"
          label="What kind of dashboard do you want?"
          placeholder="e.g. A dashboard showing cold startup times for Chrome..."
          type="text"
          fullWidth
          variant="outlined"
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          required
          multiline
          minRows={4}
          sx={{ mb: 3 }}
        />
        <Autocomplete
          multiple
          freeSolo
          options={options}
          value={selectedMetrics}
          loading={isLoading && isFocused}
          filterOptions={(x) => x}
          onFocus={() => setIsFocused(true)}
          onBlur={() => setIsFocused(false)}
          inputValue={inputValue}
          onInputChange={(_e, newInputValue) => {
            setInputValue(newInputValue);
          }}
          onChange={(_e, newValue, reason) => {
            if (
              (reason === 'selectOption' ||
                reason === 'createOption' ||
                reason === 'removeOption') &&
              Array.isArray(newValue)
            ) {
              setSelectedMetrics(newValue);
              setInputValue(''); // Clear input after selection
            }
          }}
          renderTags={(value: readonly string[], getTagProps) =>
            value.map((option: string, index: number) => {
              const { key, ...tagProps } = getTagProps({ index });
              return (
                <Chip
                  key={key}
                  variant="outlined"
                  label={option}
                  size="small"
                  {...tagProps}
                />
              );
            })
          }
          renderInput={(params) => (
            <TextField
              {...params}
              label="Associated Metric Keys"
              placeholder="Type and press Enter to add..."
              variant="outlined"
              inputProps={{
                ...params.inputProps,
                'aria-label': 'Metric Keys',
              }}
            />
          )}
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose} disabled={isPending}>
          Cancel
        </Button>
        <Button
          onClick={handleSubmit}
          variant="contained"
          disabled={!isFormValid || isPending}
          startIcon={isPending ? undefined : <AutoAwesome />}
        >
          {isPending ? 'Generating...' : 'Generate'}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
