// Copyright 2024 The LUCI Authors.
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

import { FolderOffOutlined } from '@mui/icons-material';
import { Box } from '@mui/material';

export interface NoMatchLogsProps {
  readonly secondaryText?: string;
}
export function NoMatchLog({
  secondaryText = 'Try modifying your query',
}: NoMatchLogsProps) {
  return (
    <Box
      sx={{
        display: 'flex',
        alignItems: 'center',
        flexDirection: 'column',
        justifyContent: 'center',
        color: '#757575',
      }}
    >
      <FolderOffOutlined sx={{ fontSize: '200px' }} />
      <Box sx={{ fontSize: '30px' }}>No Matching log</Box>
      <Box sx={{ fontSize: '20px' }}>{secondaryText}</Box>
    </Box>
  );
}
