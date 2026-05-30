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

import { AutoAwesome, Close as CloseIcon } from '@mui/icons-material';
import {
  Alert,
  alpha,
  Autocomplete,
  Box,
  Button,
  Chip,
  Drawer,
  IconButton,
  TextField,
  Typography,
} from '@mui/material';
import { useEffect, useState } from 'react';
import { useDebounce } from 'react-use';

import {
  AUTOCOMPLETE_DEBOUNCE_DELAY_MS,
  DATA_SPEC_ID,
  MAX_SUGGEST_RESULTS,
} from '@/crystal_ball/constants';
import { useSuggestMeasurementFilterValues } from '@/crystal_ball/hooks';

/**
 * Props for {@link GenerateDashboardDialog}.
 */
export interface GenerateDashboardDialogProps {
  /** Let the parent handle/pass down the error. */
  errorMsg?: string;
  /** Let the parent tell the dialog if it's currently saving/generating. */
  isPending?: boolean;
  /** Callback for when the modal is closed. */
  onClose: () => void;
  /** A single, unified submit handler. */
  onSubmit: (data: {
    antsInvocationId?: string;
    comparisonAntsInvocationId?: string;
    metricKeys: string[];
    prompt: string;
  }) => Promise<void>;
  /** Whether the modal is open. */
  open: boolean;
}

/**
 * A drawer component for generating a new dashboard via LLM.
 */
export function GenerateDashboardDialog({
  errorMsg = '',
  isPending = false,
  onClose,
  onSubmit,
  open,
}: GenerateDashboardDialogProps) {
  const [prompt, setPrompt] = useState('');
  const [selectedMetrics, setSelectedMetrics] = useState<string[]>([]);
  const [inputValue, setInputValue] = useState('');
  const [debouncedQuery, setDebouncedQuery] = useState('');
  const [isFocused, setIsFocused] = useState(false);
  const [antsInvocationId, setAntsInvocationId] = useState('');
  const [comparisonAntsInvocationId, setComparisonAntsInvocationId] =
    useState('');

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
      skipCache: false,
    },
    {
      enabled: open && debouncedQuery.length > 0 && isFocused,
      retry: false,
    },
  );

  const options = suggestionData?.suggestions?.map((s) => s.value) ?? [];

  const handleSubmit = async () => {
    await onSubmit({
      antsInvocationId: antsInvocationId.trim() || undefined,
      comparisonAntsInvocationId:
        comparisonAntsInvocationId.trim() || undefined,
      metricKeys: selectedMetrics,
      prompt: prompt.trim(),
    });
  };

  const isFormValid = prompt.trim().length > 0;

  // Reset state when open/closed
  useEffect(() => {
    if (open) {
      setPrompt('');
      setSelectedMetrics([]);
      setInputValue('');
      setAntsInvocationId('');
      setComparisonAntsInvocationId('');
    }
  }, [open]);

  const handleClose = () => {
    if (!isPending) {
      onClose();
    }
  };

  return (
    <Drawer
      anchor="right"
      open={open}
      onClose={handleClose}
      disableScrollLock
      sx={{
        zIndex: (theme) => theme.zIndex.modal + 10,
      }}
      PaperProps={{
        sx: {
          width: { xs: '100%', sm: 480 },
          display: 'flex',
          flexDirection: 'column',
          height: '100%',
          boxShadow: (theme) => theme.shadows[24],
        },
      }}
      ModalProps={{
        role: 'dialog',
        'aria-labelledby': 'generate-dashboard-title',
      }}
    >
      <Box
        sx={{
          p: 2.5,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
        }}
      >
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              width: 40,
              height: 40,
              borderRadius: '12px',
              bgcolor: (theme) => alpha(theme.palette.primary.main, 0.1),
              color: 'primary.main',
            }}
          >
            <AutoAwesome />
          </Box>
          <Box>
            <Typography
              id="generate-dashboard-title"
              variant="subtitle1"
              sx={{ fontWeight: 700, lineHeight: 1.2 }}
            >
              Generate Dashboard
            </Typography>
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ display: 'block', mt: 0.25 }}
            >
              Use AI to generate a customized performance dashboard
            </Typography>
          </Box>
        </Box>
        <IconButton
          onClick={handleClose}
          disabled={isPending}
          size="small"
          edge="end"
          aria-label="close"
        >
          <CloseIcon />
        </IconButton>
      </Box>

      {/* Scrollable Content */}
      <Box
        sx={{
          flex: 1,
          overflowY: 'auto',
          p: 3,
          display: 'flex',
          flexDirection: 'column',
          gap: 3,
        }}
      >
        {errorMsg && (
          <Alert severity="error" sx={{ mb: 1 }}>
            {errorMsg}
          </Alert>
        )}

        {/* Step 1: Prompt */}
        <Box>
          <Typography
            variant="caption"
            sx={{
              fontWeight: 700,
              color: 'text.secondary',
              textTransform: 'uppercase',
              letterSpacing: 1,
              mb: 1,
              display: 'block',
            }}
          >
            Step 1: Describe your dashboard
          </Typography>
          <TextField
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
            inputProps={{
              'aria-label': 'What kind of dashboard do you want?',
            }}
          />
        </Box>

        {/* Step 2: Metric Keys */}
        <Box>
          <Typography
            variant="caption"
            sx={{
              fontWeight: 700,
              color: 'text.secondary',
              textTransform: 'uppercase',
              letterSpacing: 1,
              mb: 1,
              display: 'block',
            }}
          >
            Step 2: Select Associated Metric Keys
          </Typography>
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
            onChange={(_e, newValue) => {
              if (Array.isArray(newValue)) {
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
        </Box>

        {/* Step 3: Advanced options (Invocation IDs) */}
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
          <Typography
            variant="caption"
            sx={{
              fontWeight: 700,
              color: 'text.secondary',
              textTransform: 'uppercase',
              letterSpacing: 1,
              mb: 1,
              display: 'block',
            }}
          >
            Step 3: Filter by Test Run (Optional)
          </Typography>
          <TextField
            label="Primary AnTS Invocation ID"
            placeholder="e.g. I12345..."
            type="text"
            fullWidth
            variant="outlined"
            value={antsInvocationId}
            onChange={(e) => setAntsInvocationId(e.target.value)}
            inputProps={{
              'aria-label': 'Primary AnTS Invocation ID',
            }}
          />
          <TextField
            label="Comparison AnTS Invocation ID"
            placeholder="e.g. I67890... (Optional ID for A/B comparison)"
            type="text"
            fullWidth
            variant="outlined"
            value={comparisonAntsInvocationId}
            onChange={(e) => setComparisonAntsInvocationId(e.target.value)}
            inputProps={{
              'aria-label': 'Comparison AnTS Invocation ID',
            }}
          />
        </Box>
      </Box>

      {/* Sticky Actions Footer */}
      <Box
        sx={{
          p: 2.5,
          borderTop: '1px solid',
          borderColor: 'divider',
          bgcolor: 'background.paper',
          display: 'flex',
          justifyContent: 'flex-end',
          gap: 1.5,
        }}
      >
        <Button
          onClick={handleClose}
          variant="outlined"
          color="inherit"
          disabled={isPending}
          sx={{ px: 3 }}
        >
          Cancel
        </Button>
        <Button
          onClick={handleSubmit}
          variant="contained"
          disabled={!isFormValid || isPending}
          startIcon={isPending ? undefined : <AutoAwesome />}
          sx={{
            px: 4,
            boxShadow: (theme) =>
              `0 4px 12px ${alpha(theme.palette.primary.main, 0.2)}`,
            '&:hover': {
              boxShadow: (theme) =>
                `0 6px 16px ${alpha(theme.palette.primary.main, 0.3)}`,
            },
          }}
        >
          {isPending ? 'Generating...' : 'Generate'}
        </Button>
      </Box>
    </Drawer>
  );
}
