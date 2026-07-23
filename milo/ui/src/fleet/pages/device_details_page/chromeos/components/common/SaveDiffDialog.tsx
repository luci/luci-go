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
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';

import { FieldDiff } from '../../utils/inventory_editing_utils';

interface SaveDiffDialogProps {
  open: boolean;
  saveState: 'review' | 'saving' | 'success' | 'error';
  diffs: FieldDiff[];
  onConfirm: () => void;
  onCancel: () => void;
  onClose: () => void;
  errorMessage?: string | null;
}

export const SaveDiffDialog = ({
  open,
  saveState,
  diffs,
  onConfirm,
  onCancel,
  onClose,
  errorMessage,
}: SaveDiffDialogProps) => {
  return (
    <Dialog
      open={open}
      onClose={saveState === 'saving' ? undefined : onClose}
      disableEscapeKeyDown={saveState === 'saving'}
      fullWidth
      maxWidth="sm"
    >
      {saveState === 'review' && (
        <>
          <DialogTitle sx={{ fontWeight: 'bold' }}>Review Changes</DialogTitle>
          <DialogContent dividers>
            <Typography variant="body2" sx={{ mb: 2 }} color="text.secondary">
              Please review the modifications before saving:
            </Typography>
            <TableContainer component={Paper} variant="outlined" sx={{ mb: 2 }}>
              <Table size="small" aria-label="changes diff table">
                <TableHead>
                  <TableRow>
                    <TableCell sx={{ fontWeight: 'bold', width: '150px' }}>
                      Field
                    </TableCell>
                    <TableCell sx={{ fontWeight: 'bold' }}>Original</TableCell>
                    <TableCell sx={{ fontWeight: 'bold' }}>Updated</TableCell>
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
                        }}
                      >
                        {diff.path}
                      </TableCell>
                      <TableCell
                        sx={{ wordBreak: 'break-all', verticalAlign: 'top' }}
                      >
                        {diff.original}
                      </TableCell>
                      <TableCell
                        sx={{
                          fontWeight: 'bold',
                          wordBreak: 'break-all',
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
