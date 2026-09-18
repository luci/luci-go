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
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import {
  Box,
  Button,
  ButtonBase,
  Collapse,
  Paper,
  Typography,
} from '@mui/material';
import { useEffect, useId, useRef, useState } from 'react';

import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';

import CodeSnippet from './code_snippet';

export interface MarkdownSnippetProps {
  markdown: string;
  copyKind: string;
  label?: string;
  collapsible?: boolean;
  defaultExpanded?: boolean;
}

/**
 * Shared component for displaying and copying formatted Buganizer Markdown
 * across Fleet Console dialogs (e.g., UFS inventory edits and Autorepair results).
 */
export function MarkdownSnippet({
  markdown,
  copyKind,
  label = 'Markdown for Buganizer:',
  collapsible = false,
  defaultExpanded = false,
}: MarkdownSnippetProps) {
  const { trackEvent } = useGoogleAnalytics();
  const collapseId = useId();
  const [expanded, setExpanded] = useState(defaultExpanded);
  const [copied, setCopied] = useState(false);
  const copyTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (copyTimeoutRef.current) {
        clearTimeout(copyTimeoutRef.current);
      }
    };
  }, []);

  if (!markdown) {
    return null;
  }

  const handleCopy = async () => {
    if (!navigator.clipboard) {
      return;
    }
    trackEvent('copy_code', {
      componentName: 'copy_markdown_button',
      copyKind,
    });
    try {
      await navigator.clipboard.writeText(markdown);
      setCopied(true);
      if (copyTimeoutRef.current) {
        clearTimeout(copyTimeoutRef.current);
      }
      copyTimeoutRef.current = setTimeout(() => setCopied(false), 2000);
    } catch (e) {
      // eslint-disable-next-line no-console
      console.warn('Failed to copy markdown to clipboard:', e);
    }
  };

  if (!collapsible) {
    return (
      <Box
        sx={{
          mt: 2,
          width: '100%',
          display: 'flex',
          flexDirection: 'column',
          gap: 1,
        }}
      >
        <Typography variant="body2" color="text.secondary">
          {label}
        </Typography>
        <CodeSnippet
          displayText={markdown}
          copyText={markdown}
          copyKind={copyKind}
        />
      </Box>
    );
  }

  return (
    <Paper
      variant="outlined"
      sx={{
        mt: 2,
        width: '100%',
        overflow: 'hidden',
      }}
    >
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          px: 1.5,
          py: 0.75,
        }}
      >
        <ButtonBase
          onClick={() => setExpanded((prev) => !prev)}
          aria-expanded={expanded}
          aria-controls={collapseId}
          sx={{
            display: 'flex',
            alignItems: 'center',
            gap: 1,
            flexGrow: 1,
            justifyContent: 'flex-start',
            textAlign: 'left',
            py: 0.5,
            borderRadius: 0.5,
          }}
        >
          <ExpandMoreIcon
            fontSize="small"
            sx={{
              color: 'text.secondary',
              transform: expanded ? 'rotate(180deg)' : 'rotate(0deg)',
              transition: 'transform 0.2s',
            }}
          />
          <Typography variant="body2" color="text.secondary" fontWeight={500}>
            {label}
          </Typography>
        </ButtonBase>
        <Button
          size="small"
          onClick={handleCopy}
          startIcon={copied ? <CheckIcon /> : <ContentCopyIcon />}
          sx={{ flexShrink: 0, ml: 1 }}
        >
          {copied ? 'Copied' : 'Copy markdown'}
        </Button>
      </Box>
      <Collapse in={expanded} id={collapseId} unmountOnExit>
        <Box sx={{ p: 1.5, pt: 0 }}>
          <CodeSnippet
            displayText={markdown}
            copyText={markdown}
            copyKind={copyKind}
          />
        </Box>
      </Collapse>
    </Paper>
  );
}
