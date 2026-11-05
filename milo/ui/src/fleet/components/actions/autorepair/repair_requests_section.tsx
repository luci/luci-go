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

import {
  Box,
  Button,
  Checkbox,
  FormControl,
  FormControlLabel,
  FormLabel,
  Stack,
  Typography,
} from '@mui/material';

import { REPAIR_REQUEST_OPTIONS, RepairRequestAction } from './shared';

export interface RepairRequestsSectionProps {
  readonly repairRequests: readonly RepairRequestAction[];
  readonly onChange?: (next: RepairRequestAction[]) => void;
}

export function RepairRequestsSection({
  repairRequests,
  onChange,
}: RepairRequestsSectionProps) {
  const toggleRepairRequest = (action: RepairRequestAction) => {
    if (!onChange) return;
    const exists = repairRequests.includes(action);
    const next = exists
      ? repairRequests.filter((a) => a !== action)
      : [...repairRequests, action];
    const canonicalOrder = REPAIR_REQUEST_OPTIONS.map((o) => o.value).filter(
      (v) => next.includes(v),
    );
    onChange(canonicalOrder);
  };

  return (
    <FormControl
      component="fieldset"
      variant="standard"
      sx={{ display: 'block', mb: 1.5 }}
    >
      <Stack
        direction="row"
        justifyContent="space-between"
        alignItems="center"
        sx={{ mb: 0.5 }}
      >
        <FormLabel
          component="legend"
          sx={{
            color: 'text.primary',
            fontWeight: 600,
            fontSize: '14px',
            '&.Mui-focused': { color: 'text.primary' },
          }}
        >
          Place repair requests (optional)
        </FormLabel>
        <Button
          variant="text"
          size="small"
          onClick={() => onChange?.([])}
          disabled={repairRequests.length === 0}
          sx={{ minWidth: 'auto', px: 1, py: 0, textTransform: 'none' }}
        >
          Reset
        </Button>
      </Stack>
      <Box
        sx={{
          display: 'grid',
          gridTemplateColumns: '1fr 1fr',
          columnGap: 1,
          rowGap: 0.25,
          ml: 0.5,
        }}
      >
        {REPAIR_REQUEST_OPTIONS.map((option) => {
          const checked = repairRequests.includes(option.value);
          return (
            <FormControlLabel
              key={option.value}
              sx={{
                alignItems: 'center',
                mr: 0,
                my: 0.25,
              }}
              control={
                <Checkbox
                  size="small"
                  checked={checked}
                  onChange={() => toggleRepairRequest(option.value)}
                  inputProps={{
                    'aria-label': `${option.label} (${option.value})`,
                  }}
                />
              }
              label={
                <Typography
                  component="span"
                  sx={{
                    fontSize: '13.5px',
                    lineHeight: 1.25,
                    color: 'text.primary',
                  }}
                >
                  {option.label}{' '}
                  <Typography
                    component="span"
                    sx={{ fontSize: '11.5px', color: 'text.secondary' }}
                  >
                    (
                    <Box
                      component="code"
                      sx={{
                        fontFamily: 'Roboto Mono, monospace',
                        fontSize: '11px',
                      }}
                    >
                      {option.value}
                    </Box>
                    )
                  </Typography>
                </Typography>
              }
            />
          );
        })}
      </Box>
    </FormControl>
  );
}
