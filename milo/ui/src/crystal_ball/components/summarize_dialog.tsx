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

import {
  AutoAwesome as AutoAwesomeIcon,
  Close as CloseIcon,
} from '@mui/icons-material';
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Drawer,
  IconButton,
  TextField,
  Typography,
} from '@mui/material';
import { useState } from 'react';

import { SanitizedHtml } from '@/common/components/sanitized_html';
import { renderMarkdown } from '@/common/tools/markdown/utils';

export interface SummarizeDialogProps {
  /**
   * Whether the dialog is open.
   */
  readonly open: boolean;
  /**
   * Callback fired when the dialog is closed.
   */
  readonly onClose: () => void;
  /**
   * The title of the dialog.
   */
  readonly title: string;
  /**
   * Callback to perform the summarization.
   * @param prompt - The prompt to guide the summary.
   * @returns A promise resolving to the summary text.
   */
  readonly onSummarize: (prompt: string) => Promise<string>;
  /**
   * The mode of the dialog (terminology and labels).
   */
  readonly mode?: 'summarize' | 'analyze';
}

/**
 * A drawer component that accepts optional instructions for AI,
 * triggers an analysis action, displays progress, and renders the result.
 */
export function SummarizeDialog({
  open,
  onClose,
  title,
  onSummarize,
  mode = 'summarize',
}: SummarizeDialogProps) {
  const [prompt, setPrompt] = useState('');
  const [summary, setSummary] = useState<string | null>(null);
  const [isPending, setIsPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const isAnalyze = mode === 'analyze';

  const handleCopy = async () => {
    if (!summary) return;
    try {
      await navigator.clipboard.writeText(summary);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error('Failed to copy text: ', err);
    }
  };

  const handleSummarize = async () => {
    setIsPending(true);
    setError(null);
    try {
      const result = await onSummarize(prompt);
      setSummary(result);
    } catch (e: unknown) {
      const err = e instanceof Error ? e.message : String(e);
      setError(
        err || (isAnalyze ? 'Failed to analyze' : 'Failed to summarize'),
      );
    } finally {
      setIsPending(false);
    }
  };

  const handleClose = () => {
    onClose();
  };

  return (
    <Drawer
      anchor="right"
      open={open}
      onClose={handleClose}
      disableScrollLock
      sx={{
        zIndex: (theme) => theme.zIndex.modal + 10,
      }}
      PaperProps={{
        sx: {
          width: { xs: '100%', sm: 550 },
          display: 'flex',
          flexDirection: 'column',
          height: '100%',
          backgroundImage: 'none',
        },
      }}
    >
      {/* Header */}
      <Box
        sx={{
          p: 2,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          borderBottom: '1px solid',
          borderColor: 'divider',
        }}
      >
        <Typography variant="h6" component="h2" sx={{ fontWeight: 'medium' }}>
          {title}
        </Typography>
        <IconButton onClick={handleClose} size="small">
          <CloseIcon />
        </IconButton>
      </Box>

      {/* Content */}
      <Box sx={{ flexGrow: 1, overflowY: 'auto', p: 3 }}>
        {isPending ? (
          <Box
            sx={{
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              justifyContent: 'center',
              height: '100%',
              gap: 2,
            }}
          >
            <CircularProgress />
            <Typography variant="body2" color="text.secondary">
              {isAnalyze ? 'Analyzing...' : 'Generating summary...'}
            </Typography>
          </Box>
        ) : summary ? (
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <Alert severity="success" sx={{ mb: 0.5 }}>
              {isAnalyze
                ? 'Analysis completed successfully.'
                : 'Summary completed successfully.'}
            </Alert>
            <Typography
              variant="subtitle2"
              color="text.secondary"
              sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}
            >
              <AutoAwesomeIcon sx={{ color: '#9c27b0', fontSize: '1.2rem' }} />
              {isAnalyze ? 'AI-Generated Analysis:' : 'AI-Generated Summary:'}
            </Typography>
            <Box
              sx={{
                bgcolor: 'action.hover',
                p: 2.5,
                borderRadius: 2,
                border: '1px solid',
                borderColor: 'divider',
              }}
            >
              <SanitizedHtml html={renderMarkdown(summary)} />
            </Box>
          </Box>
        ) : (
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2.5 }}>
            <Typography variant="body2" color="text.secondary">
              {isAnalyze
                ? 'Provide instructions/guidance to the AI for this analysis, or leave it blank for a general analysis.'
                : 'Provide instructions/guidance to the AI for this summary, or leave it blank for a general summary.'}
            </Typography>
            <TextField
              label="AI Guidance Instructions (optional)"
              placeholder="e.g. Focus on regressions in the last 2 weeks"
              value={prompt}
              onChange={(e) => setPrompt(e.target.value)}
              fullWidth
              multiline
              rows={4}
              variant="outlined"
            />
            {error && (
              <Typography color="error" variant="body2">
                {error}
              </Typography>
            )}
          </Box>
        )}
      </Box>

      {/* Footer Actions */}
      <Box
        sx={{
          p: 2,
          display: 'flex',
          justifyContent: 'space-between',
          borderTop: '1px solid',
          borderColor: 'divider',
          bgcolor: 'background.default',
        }}
      >
        <Button onClick={handleClose}>{summary ? 'Close' : 'Cancel'}</Button>
        <Box sx={{ display: 'flex', gap: 1 }}>
          {summary && (
            <>
              <Button
                variant="outlined"
                onClick={handleCopy}
                disabled={isPending}
              >
                {copied ? 'Copied!' : 'Copy to Clipboard'}
              </Button>
              <Button
                variant="outlined"
                onClick={() => setSummary(null)}
                disabled={isPending}
              >
                Ask again
              </Button>
            </>
          )}
          {!summary && (
            <Button
              variant="contained"
              onClick={handleSummarize}
              disabled={isPending}
              sx={{
                bgcolor: '#9c27b0',
                '&:hover': {
                  bgcolor: '#7b1fa2',
                },
              }}
            >
              {isAnalyze ? 'Analyze' : 'Summarize'}
            </Button>
          )}
        </Box>
      </Box>
    </Drawer>
  );
}
