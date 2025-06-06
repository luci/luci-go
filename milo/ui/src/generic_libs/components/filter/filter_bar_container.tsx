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

import ClearIcon from '@mui/icons-material/Clear';
import { Box, IconButton, Typography } from '@mui/material';

interface FilterBarContainerProps {
  children: React.ReactNode;
  showClearAll?: boolean; // Optional: To show the clear all button
  onClearAll?: () => void; // Optional: Callback for the clear all button
}

/**
 * A layout component that provides the visual structure for a filter bar.
 * It includes the "Filter" label, borders, and an optional "clear all" button.
 */
export function FilterBarContainer({
  children,
  showClearAll,
  onClearAll,
}: FilterBarContainerProps) {
  return (
    <Box
      sx={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between', // Pushes clear button to the right
        gap: 1,
        p: 0,
      }}
    >
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
        <Typography variant="body2" sx={{ mr: 1, whiteSpace: 'nowrap' }}>
          Filter:
        </Typography>
        {children}
      </Box>
      {showClearAll && onClearAll && (
        <IconButton
          onClick={onClearAll}
          size="small"
          aria-label="clear all filters"
        >
          <ClearIcon fontSize="small" />
        </IconButton>
      )}
    </Box>
  );
}
