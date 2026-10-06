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

import CheckIcon from '@mui/icons-material/Check';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import { Box, IconButton, Link, Tooltip, Typography } from '@mui/material';
import { MouseEvent, useState } from 'react';

export interface CodeChipProps {
  value: string | null | undefined;
  href?: string;
}

export const CodeChip = ({ value, href }: CodeChipProps) => {
  const [copied, setCopied] = useState(false);
  if (!value) return null;

  const handleCopy = (e: MouseEvent) => {
    e.stopPropagation();
    e.preventDefault();
    navigator.clipboard?.writeText(value);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  const textStyle = {
    fontFamily: 'monospace',
    fontWeight: 600,
    wordBreak: 'break-word',
  } as const;

  return (
    <Box
      className="code-chip-root"
      sx={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 0.5,
        maxWidth: '100%',
        '&:hover .code-chip-copy-btn, &:focus-within .code-chip-copy-btn': {
          opacity: 1,
        },
      }}
    >
      {href ? (
        <Link
          href={href}
          target="_blank"
          rel="noreferrer"
          variant="body2"
          sx={{
            ...textStyle,
            textDecoration: 'none',
            '&:hover': { textDecoration: 'underline' },
          }}
        >
          {value}
        </Link>
      ) : (
        <Typography component="span" variant="body2" sx={textStyle}>
          {value}
        </Typography>
      )}
      {value !== 'N/A' && (
        <Tooltip title={copied ? 'Copied!' : `Copy ${value}`}>
          <IconButton
            className="code-chip-copy-btn"
            size="small"
            aria-label={`Copy ${value}`}
            onClick={handleCopy}
            sx={{
              p: 0.25,
              borderRadius: 1,
              color: copied ? 'success.main' : 'text.secondary',
              opacity: copied ? 1 : 0.55,
              transition: 'opacity 0.15s ease, background-color 0.15s ease',
              '&:hover': {
                opacity: 1,
                color: copied ? 'success.main' : 'primary.main',
                bgcolor: 'action.hover',
              },
            }}
          >
            {copied ? (
              <CheckIcon sx={{ fontSize: 14 }} />
            ) : (
              <ContentCopyIcon sx={{ fontSize: 14 }} />
            )}
          </IconButton>
        </Tooltip>
      )}
    </Box>
  );
};
