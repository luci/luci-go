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
import { Button, Tooltip, Zoom } from '@mui/material';

interface CopyButtonProps {
  onClick: () => void;
}

/**
 * A button that, when clicked, displays a tooltip instructing the user to
 * press Ctrl+C to copy the selected rows in a spreadsheet-friendly format.
 */
export function CopyButton({ onClick }: CopyButtonProps) {
  return (
    <Tooltip
      title={'Copy to clipboard (Ctrl + C)'}
      arrow
      TransitionComponent={Zoom}
      placement="top"
    >
      <Button
        onClick={onClick}
        startIcon={<ContentCopyIcon />}
        aria-label="Copy selected rows"
      >
        Copy
      </Button>
    </Tooltip>
  );
}
