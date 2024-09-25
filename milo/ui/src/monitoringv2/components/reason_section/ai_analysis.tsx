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

import AutoAwesomeIcon from '@mui/icons-material/AutoAwesome';
import {
  Alert,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  Grid,
  IconButton,
  LinearProgress,
  LinearProgressProps,
  Typography,
} from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useMemo, useState } from 'react';

import { useTestVariantBranchesClient } from '@/analysis/hooks/prpc_clients';
import { SanitizedHtml } from '@/common/components/sanitized_html';
import { logging } from '@/common/tools/logging';
import { renderMarkdown } from '@/common/tools/markdown/utils';
import { CopyToClipboard } from '@/generic_libs/components/copy_to_clipboard';
import { AlertReasonTestJson } from '@/monitoringv2/util/server_json';

interface PromptDetailsProps {
  readonly prompt: string;
}

function PromptDetails({ prompt }: PromptDetailsProps) {
  return (
    <>
      <h3>
        Prompt:
        <CopyToClipboard textToCopy={prompt || ''} title="Copy prompt" />
      </h3>
      <Grid
        container
        sx={{
          maxHeight: '400px',
          overflowY: 'scroll',
          mb: 1,
          px: 1,
          overflowWrap: 'break-word',
          backgroundColor: 'var(--block-background-color)',
        }}
      >
        <pre>{prompt}</pre>
      </Grid>
      <Divider orientation="horizontal" />
    </>
  );
}

function LinearProgressWithLabel(
  props: LinearProgressProps & { labelString: string },
) {
  return (
    <Box sx={{ display: 'flex', alignItems: 'center' }}>
      <Box sx={{ minWidth: 35, mr: 1 }}>
        <Typography variant="body2" color="text.secondary">
          {props.labelString}
        </Typography>
      </Box>
      <Box sx={{ width: '100%' }}>
        <LinearProgress />
      </Box>
    </Box>
  );
}

interface AIAnalysisProps {
  test: AlertReasonTestJson;
  project: string;
}

export function AIAnalysis({ test, project }: AIAnalysisProps) {
  const client = useTestVariantBranchesClient();
  const [open, setOpen] = useState(false);
  const [showPrompt, setShowPrompt] = useState(false);
  logging.warn(test.regression_start_position);
  const {
    data: aiData,
    refetch: refetchAIAnalysis,
    isLoading,
    isError,
  } = useQuery({
    ...client.QueryChangepointAIAnalysis.query({
      project: project,
      refHash: test.ref_hash,
      testId: test.test_id,
      variantHash: test.variant_hash,
      startSourcePosition: String(test.regression_start_position),
      promptOptions: {
        prefix: '',
        suffix: '',
      },
    }),
    enabled: false,
    refetchOnMount: false,
    refetchOnWindowFocus: false,
  });

  const aiHTML = useMemo(
    () =>
      aiData?.analysisMarkdown ? renderMarkdown(aiData.analysisMarkdown) : null,
    [aiData],
  );

  const handleClickOpen = () => {
    if (!aiData) {
      refetchAIAnalysis();
      setShowPrompt(false);
    }
    setOpen(true);
  };

  const handleClose = () => {
    if (isLoading) {
      return;
    }
    setOpen(false);
  };

  const handleTogglePrompt = () => {
    setShowPrompt(!showPrompt);
  };

  return (
    <>
      <IconButton
        title="Analyze culprit"
        color="primary"
        aria-label="delete"
        onClick={handleClickOpen}
      >
        <AutoAwesomeIcon />
      </IconButton>
      <Dialog
        onClose={handleClose}
        fullWidth
        open={open}
        scroll="paper"
        maxWidth="xl"
        aria-labelledby="scroll-dialog-title"
        aria-describedby="scroll-dialog-description"
      >
        <DialogTitle id="scroll-dialog-title">AI cuplrit analysis</DialogTitle>
        <DialogContent dividers>
          {isLoading && <LinearProgressWithLabel labelString="Analyzing" />}
          {showPrompt && aiData && <PromptDetails prompt={aiData.prompt} />}
          {aiHTML && <SanitizedHtml html={aiHTML} />}
          {isError && <Alert severity="error">{'Failed to load'}</Alert>}
        </DialogContent>
        {aiData?.prompt && (
          <DialogActions>
            <Button onClick={handleTogglePrompt}>Toggle prompt</Button>
          </DialogActions>
        )}
      </Dialog>
    </>
  );
}
