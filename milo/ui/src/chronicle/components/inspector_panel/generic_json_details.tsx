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

import { Box, Paper, Typography } from '@mui/material';

export interface GenericJsonDetailsProps {
  json?: string;
  typeUrl?: string;
}

export function GenericJsonDetails({ json, typeUrl }: GenericJsonDetailsProps) {
  if (!json) return null;

  const prettyPrintedJson = JSON.stringify(JSON.parse(json), null, 2);

  return (
    <Box sx={{ mt: 1 }}>
      {typeUrl && (
        <Typography variant="caption" color="text.secondary">
          {typeUrl}
        </Typography>
      )}
      <Paper
        variant="outlined"
        sx={{
          p: 1,
          bgcolor: '#f5f5f5',
          maxHeight: '200px',
          overflow: 'auto',
          fontSize: '0.75rem',
          fontFamily: 'monospace',
        }}
      >
        <pre
          style={{ margin: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}
        >
          {prettyPrintedJson}
        </pre>
      </Paper>
    </Box>
  );
}
