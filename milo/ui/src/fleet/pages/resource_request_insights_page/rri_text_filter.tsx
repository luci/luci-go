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

import { Box, TextField } from '@mui/material';
import { useEffect } from 'react';

import { OptionComponentProps } from '@/fleet/components/filter_dropdown/filter_dropdown';

import { ResourceRequestInsightsOptionComponentProps } from './resource_request_insights_page';

export function RriTextFilter({
  optionComponentProps: { filters, onClose, onFiltersChange, option, onApply },
}: OptionComponentProps<ResourceRequestInsightsOptionComponentProps>) {
  useEffect(() => () => onClose(), [onClose]);

  const handleKeyDown = (event: React.KeyboardEvent) => {
    if (event.key !== 'Escape') {
      event.stopPropagation();
    }
    if (event.key === 'Enter') {
      onApply();
    }
  };

  const currentValue = filters ? filters[option.value] : '';

  return (
    <Box sx={{ padding: 2, display: 'flex', gap: 1, alignItems: 'center' }}>
      <TextField
        label={`Filter by ${option.label}`}
        variant="outlined"
        size="small"
        value={currentValue}
        onChange={(e) =>
          onFiltersChange({ ...filters, [option.value]: e.target.value })
        }
        onKeyDown={handleKeyDown}
        fullWidth
        sx={{ minWidth: '200px' }}
        // eslint-disable-next-line jsx-a11y/no-autofocus
        autoFocus
      />
    </Box>
  );
}
