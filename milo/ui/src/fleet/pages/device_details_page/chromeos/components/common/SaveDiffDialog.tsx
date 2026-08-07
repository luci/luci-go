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

import { CheckCircleOutline as CheckCircleOutlineIcon } from '@mui/icons-material';
import {
  Alert,
  AlertTitle,
  Box,
  Button,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Link,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';

import CodeSnippet from '@/fleet/components/code_snippet/code_snippet';
import { Status } from '@/proto/google/rpc/status.pb';

import {
  FieldDiff,
  generateChangelogMarkdown,
} from '../../utils/inventory_editing_utils';

interface SaveDiffDialogProps {
  open: boolean;
  saveState: 'review' | 'saving' | 'success' | 'error';
  diffs: FieldDiff[];
  shivasCommands: string[];
  deviceId: string;
  onConfirm: () => void;
  onCancel: () => void;
  onClose: () => void;
  onExited?: () => void;
  errorMessage?: string | null;
  hasDeployableEdits?: boolean;
  deployTaskUrl?: string;
  deployTaskStatus?: Status | null;
}

export const SaveDiffDialog = ({
  open,
  saveState,
  diffs,
  shivasCommands,
  deviceId,
  onConfirm,
  onCancel,
  onClose,
  onExited,
  errorMessage,
  hasDeployableEdits = false,
  deployTaskUrl,
  deployTaskStatus,
}: SaveDiffDialogProps) => {
  const changelogMarkdown = generateChangelogMarkdown(diffs, deviceId);

  const hasDeployError = Boolean(
    deployTaskStatus &&
      deployTaskStatus.code !== undefined &&
      deployTaskStatus.code !== 0,
  );

  return (
    <Dialog
      open={open}
      onClose={saveState === 'saving' ? undefined : onClose}
      disableEscapeKeyDown={saveState === 'saving'}
      TransitionProps={{ onExited }}
      fullWidth
      maxWidth="sm"
    >
      {saveState === 'review' && (
        <>
          <DialogTitle sx={{ fontWeight: 'bold' }}>Review Changes</DialogTitle>
          <DialogContent dividers sx={{ p: 2.5, overflowX: 'hidden' }}>
            {hasDeployableEdits && (
              <Alert severity="warning" sx={{ mb: 2 }}>
                <AlertTitle>Redeployment Required</AlertTitle>
                Device will be locked and a deploy task will be run. No other
                tasks will be scheduled until deployment completes.
              </Alert>
            )}
            <Typography variant="body2" sx={{ mb: 2 }} color="text.secondary">
              Please review the modifications before saving:
            </Typography>
            <TableContainer component={Paper} variant="outlined" sx={{ mb: 2 }}>
              <Table
                size="small"
                aria-label="changes diff table"
                sx={{ tableLayout: 'fixed', width: '100%' }}
              >
                <TableHead>
                  <TableRow>
                    <TableCell sx={{ fontWeight: 'bold', width: '25%' }}>
                      Field
                    </TableCell>
                    <TableCell sx={{ fontWeight: 'bold', width: '37.5%' }}>
                      Original
                    </TableCell>
                    <TableCell sx={{ fontWeight: 'bold', width: '37.5%' }}>
                      Updated
                    </TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {diffs.map((diff, index) => (
                    <TableRow key={`${diff.path}-${index}`}>
                      <TableCell
                        sx={{
                          fontFamily: 'monospace',
                          color: 'text.secondary',
                          verticalAlign: 'top',
                          overflowWrap: 'break-word',
                        }}
                      >
                        {diff.path}
                      </TableCell>
                      <TableCell
                        sx={{
                          overflowWrap: 'break-word',
                          verticalAlign: 'top',
                        }}
                      >
                        {diff.original}
                      </TableCell>
                      <TableCell
                        sx={{
                          fontWeight: 'bold',
                          overflowWrap: 'break-word',
                          verticalAlign: 'top',
                        }}
                      >
                        {diff.updated}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>

            {shivasCommands.length > 0 && (
              <Box sx={{ mt: 2 }}>
                <Typography
                  variant="body2"
                  sx={{ mb: 1 }}
                  color="text.secondary"
                >
                  {`Equivalent shivas command${shivasCommands.length > 1 ? 's' : ''}:`}
                </Typography>
                <Box sx={{ mb: 1 }}>
                  <CodeSnippet
                    displayText={shivasCommands.join(' && \\\n')}
                    copyText={shivasCommands.join(' && \\\n')}
                    copyKind="shivas_command"
                  />
                </Box>
              </Box>
            )}
          </DialogContent>
          <DialogActions>
            <Button onClick={onCancel} color="inherit">
              Cancel
            </Button>
            <Button onClick={onConfirm} variant="contained" color="primary">
              Confirm & Save
            </Button>
          </DialogActions>
        </>
      )}

      {saveState === 'saving' && (
        <DialogContent
          sx={{
            py: 6,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            gap: 2,
          }}
        >
          <CircularProgress size={40} />
          <Typography variant="body2" color="text.secondary">
            Saving changes to UFS service...
          </Typography>
        </DialogContent>
      )}

      {saveState === 'success' && (
        <DialogContent
          sx={{
            py: 6,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            gap: 2,
          }}
        >
          <CheckCircleOutlineIcon color="success" sx={{ fontSize: 60 }} />
          <Typography variant="subtitle1" sx={{ fontWeight: 'bold' }}>
            Changes Saved Successfully
          </Typography>
          <Typography variant="body2" color="text.secondary" align="center">
            Your inventory modifications have been pushed to UFS.
          </Typography>

          {hasDeployError && (
            <Alert severity="warning" sx={{ mt: 2, width: '100%' }}>
              <AlertTitle>Deploy Task Scheduling Failed</AlertTitle>
              {deployTaskStatus?.message ||
                'Failed to automatically schedule deployment task.'}{' '}
              Please re-run the deploy task manually.
              {deployTaskUrl && (
                <>
                  {' '}
                  <Link
                    href={deployTaskUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                    underline="always"
                  >
                    View deploy task
                  </Link>
                </>
              )}
            </Alert>
          )}

          {!hasDeployError && deployTaskUrl && (
            <Alert severity="info" sx={{ mt: 2, width: '100%' }}>
              <AlertTitle>Deploy Task Scheduled</AlertTitle>A deploy task has
              been scheduled to verify hardware changes.{' '}
              <Link
                href={deployTaskUrl}
                target="_blank"
                rel="noopener noreferrer"
                underline="always"
              >
                View deploy task
              </Link>
            </Alert>
          )}

          {!hasDeployError && !deployTaskUrl && hasDeployableEdits && (
            <Alert severity="info" sx={{ mt: 2, width: '100%' }}>
              <AlertTitle>Redeployment Required</AlertTitle>
              Your changes were saved. A deploy task may be required to verify
              hardware changes.
            </Alert>
          )}

          {changelogMarkdown && (
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
                Changelog (Markdown for Buganizer):
              </Typography>
              <CodeSnippet
                displayText={changelogMarkdown}
                copyText={changelogMarkdown}
                copyKind="changelog"
              />
            </Box>
          )}

          <Box sx={{ mt: 2 }}>
            <Button onClick={onClose} variant="contained" color="primary">
              Close
            </Button>
          </Box>
        </DialogContent>
      )}

      {saveState === 'error' && (
        <>
          <DialogTitle>Error Saving Changes</DialogTitle>
          <DialogContent>
            <Alert severity="error" sx={{ mt: 1 }}>
              <AlertTitle>Failed to write updates to UFS</AlertTitle>
              {errorMessage ||
                'An unexpected error occurred. Please try again.'}
            </Alert>
          </DialogContent>
          <DialogActions>
            <Button onClick={onClose} color="primary" variant="contained">
              Close
            </Button>
          </DialogActions>
        </>
      )}
    </Dialog>
  );
};
