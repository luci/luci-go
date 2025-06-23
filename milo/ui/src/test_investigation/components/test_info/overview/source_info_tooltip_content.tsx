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

import { Box, Button, Typography } from '@mui/material';

import { TestLocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_metadata.pb';

interface SourceInfoTooltipContentProps {
  testLocation?: TestLocation;
  sourceRef?: string | null;
  codesearchUrl?: string | null;
}

export function SourceInfoTooltipContent({
  testLocation,
  sourceRef,
  codesearchUrl,
}: SourceInfoTooltipContentProps) {
  return (
    <Box sx={{ p: 1 }}>
      <Box
        sx={{
          display: 'flex',
          mb: 0.5,
          alignItems: 'center',
        }}
      >
        <Typography sx={{ fontWeight: 'medium' }}>Test Case Source</Typography>
      </Box>

      {testLocation?.fileName && (
        <TooltipRow label="File">{testLocation.fileName}</TooltipRow>
      )}
      {!!testLocation?.line && (
        <TooltipRow label="Line">{testLocation.line}</TooltipRow>
      )}
      {testLocation?.repo && (
        <TooltipRow label="Repo">{testLocation.repo}</TooltipRow>
      )}
      {sourceRef && <TooltipRow label="Ref">{sourceRef}</TooltipRow>}
      {codesearchUrl && (
        <Button
          variant="outlined"
          size="small"
          href={codesearchUrl}
          target="_blank"
          rel="noopener noreferrer"
        >
          Codesearch
        </Button>
      )}
    </Box>
  );
}

interface TooltipRowProps {
  label: string;
  children: React.ReactNode;
}

function TooltipRow({ label, children }: TooltipRowProps) {
  return (
    <Box
      sx={{
        display: 'flex',
        mb: 0.5,
        alignItems: 'flex-start',
      }}
    >
      <Typography
        variant="caption"
        sx={{
          fontWeight: 'medium',
          color: 'text.secondary',
          mr: 1,
          whiteSpace: 'nowrap',
        }}
      >
        {label}:
      </Typography>
      <Typography
        variant="body2"
        sx={{ color: 'text.primary', wordBreak: 'break-all' }}
      >
        {children}
      </Typography>
    </Box>
  );
}
