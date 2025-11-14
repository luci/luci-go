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

import { Box, Tooltip } from '@mui/material';
import { ReactNode } from 'react';

interface TextBoxProps {
  lines: string[];
  emphasized?: boolean;
  tooltip?: ReactNode;
}

export function TextBox({ lines, emphasized, tooltip }: TextBoxProps) {
  const content = (
    <Box
      component="pre"
      sx={{
        whiteSpace: 'pre-wrap',
        wordBreak: 'break-all',
        p: '0 1rem',
        m: 0,
        fontSize: '0.8rem',
        lineHeight: '1.15rem',
        color: emphasized ? '#cc0000' : 'text.primary',
      }}
    >
      {lines.join('\n')}
    </Box>
  );
  if (tooltip) {
    return (
      <Tooltip title={tooltip} placement="left" enterDelay={1500} arrow>
        {content}
      </Tooltip>
    );
  }
  return content;
}
