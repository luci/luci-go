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

import { Box, Typography } from '@mui/material';

import { CopyToClipboard } from '@/generic_libs/components/copy_to_clipboard';

interface PageTitleProps {
  viewName: string;
  resourceName: string;
}

/**
 * Renders a standard page title with the view name and resource name, and a copy to clipboard button for the resource name.
 *
 * Format is "viewName: resourceName [copy icon]"
 */
export function PageTitle({ viewName, resourceName }: PageTitleProps) {
  return (
    <Box sx={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap' }}>
      <Typography
        variant="h5"
        component="h1"
        sx={{ color: 'var(--gm3-color-on-surface-strong)', mr: 0.5 }}
      >
        {viewName}:
      </Typography>
      <Typography
        variant="h5"
        component="span"
        sx={{
          color: 'var(--gm3-color-on-surface-strong)',
          wordBreak: 'break-all',
        }}
      >
        {resourceName}
      </Typography>
      <CopyToClipboard
        textToCopy={resourceName}
        aria-label="Copy to clipboard."
        sx={{ ml: 0.5 }}
      />
    </Box>
  );
}
