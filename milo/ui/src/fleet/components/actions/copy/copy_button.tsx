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

import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import { Button, Tooltip } from '@mui/material';
import { useState } from 'react';

/**
 * A button that, when clicked, displays a tooltip instructing the user to
 * press Ctrl+C to copy the selected rows in a spreadsheet-friendly format.
 */
export function CopyButton() {
  const [tooltipOpen, setTooltipOpen] = useState(false);
  return (
    <Tooltip
      open={tooltipOpen}
      onClose={() => setTooltipOpen(false)}
      title="Press Ctrl+C to copy selected rows"
      leaveDelay={1000}
    >
      <Button
        onClick={() => setTooltipOpen(true)}
        startIcon={<ContentCopyIcon />}
        aria-label="Copy selected rows"
      >
        Copy
      </Button>
    </Tooltip>
  );
}
