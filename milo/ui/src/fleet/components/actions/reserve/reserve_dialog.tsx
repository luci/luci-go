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

import HelpOutlineIcon from '@mui/icons-material/HelpOutline';
import {
  Box,
  Button,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  Stack,
  TextField,
  Tooltip,
} from '@mui/material';
import { useState, useEffect } from 'react';

import { generateChromeOsDeviceDetailsURL } from '@/fleet/constants/paths';
import { FLEET_BUILDS_SWARMING_HOST } from '@/fleet/utils/builds';

import CodeSnippet from '../../code_snippet/code_snippet';

export interface ReserveResult {
  unitName: string;
  success: boolean;
  redirectUrl?: string;
  errorMessage?: string;
}

export interface ReserveDialogProps {
  open: boolean;
  sessionInfo: {
    dutNames?: string[];
    results?: ReserveResult[];
    sessionId?: string;
  };
  handleClose: () => void;
  handleOk: () => void;
  loading: boolean;
  comment: string;
  handleCommentChange: (comment: string) => void;
}

const plurifyDevices = (count: number) => {
  return count === 1 ? 'device' : `${count} devices`;
};

function getDeviceDetailListItem(dutName: string) {
  return (
    <li key={dutName}>
      <a
        href={generateChromeOsDeviceDetailsURL(dutName)}
        target="_blank"
        rel="noreferrer"
      >
        {dutName}
      </a>
    </li>
  );
}

export default function ReserveDialog({
  open,
  sessionInfo: { dutNames = [], results, sessionId },
  handleClose,
  handleOk,
  loading,
  comment,
  handleCommentChange,
}: ReserveDialogProps) {
  const [touched, setTouched] = useState<boolean>(false);

  useEffect(() => {
    if (!open) {
      setTouched(false);
    }
  }, [open]);

  const escapedComment = comment.replace(/\\/g, '\\\\').replace(/"/g, '\\"');
  const shivasCommand = `shivas reserve-duts -comment "${escapedComment}" ${dutNames.join(' ')}`;

  const loadingScreen = (
    <>
      <DialogTitle>Reserving devices</DialogTitle>
      <DialogContent>
        <Box
          sx={{
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            minHeight: '100px',
          }}
        >
          <CircularProgress />
        </Box>
      </DialogContent>
    </>
  );

  const confirmationScreen = (
    <>
      <DialogTitle>Reserving devices</DialogTitle>
      <DialogContent>
        {dutNames.length > 0 && (
          <>
            <p>
              Please confirm that you want to reserve the following{' '}
              {plurifyDevices(dutNames.length)}:
            </p>
            <ul>
              {dutNames.map((dutName) => getDeviceDetailListItem(dutName))}
            </ul>
            <Box sx={{ mt: 2, mb: 2 }}>
              <TextField
                label="Comment"
                placeholder="Reason for reserving"
                variant="outlined"
                fullWidth
                required
                value={comment}
                onChange={(e) => {
                  setTouched(true);
                  handleCommentChange(e.target.value);
                }}
                error={touched && comment.trim() === ''}
                helperText={
                  touched && comment.trim() === '' ? 'Comment is required' : ' '
                }
              />
            </Box>

            <Stack direction="row" alignItems="center" spacing={1}>
              <p>Equivalent shivas command:</p>
              <Tooltip title="View documentation">
                <IconButton
                  size="small"
                  href="http://go/shivas-manual-os"
                  target="_blank"
                >
                  <HelpOutlineIcon fontSize="small" />
                </IconButton>
              </Tooltip>
            </Stack>
            <CodeSnippet
              displayText={shivasCommand}
              copyText={shivasCommand}
              copyKind="shivas_reserve"
            />
          </>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose}>Cancel</Button>
        {dutNames.length > 0 && (
          <Button
            onClick={handleOk}
            variant="contained"
            disabled={comment.trim() === ''}
          >
            Confirm
          </Button>
        )}
      </DialogActions>
    </>
  );

  const finalScreen = (
    <>
      <DialogTitle>Reserve results</DialogTitle>
      <DialogContent>
        <p>
          Reserve has been triggered on the following{' '}
          {plurifyDevices(results?.length || 0)}:
        </p>
        <ul>
          {results?.map((result) => {
            return (
              <li key={result.unitName}>
                <a
                  href={generateChromeOsDeviceDetailsURL(result.unitName)}
                  target="_blank"
                  rel="noreferrer"
                >
                  {result.unitName}
                </a>
                {': '}
                {result.success && result.redirectUrl ? (
                  <a href={result.redirectUrl} target="_blank" rel="noreferrer">
                    View in Milo
                  </a>
                ) : (
                  <span style={{ color: 'red' }}>
                    Failed to schedule reserve:{' '}
                    {result.errorMessage || 'Unknown error'}
                  </span>
                )}
              </li>
            );
          })}
        </ul>
        {!!sessionId && (
          <>
            <p>
              (
              <a
                href={`https://${FLEET_BUILDS_SWARMING_HOST}/tasklist?f=admin-session:${sessionId}`}
                target="_blank"
                rel="noreferrer"
              >
                View tasks in Swarming
              </a>
              )
            </p>
            <p>
              <i>
                It may take a few minutes for Swarming to update the task to
                show up on Milo.
              </i>
            </p>
          </>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose} variant="contained">
          Done
        </Button>
      </DialogActions>
    </>
  );

  const getDialogContent = () => {
    if (loading) {
      return loadingScreen;
    }
    if (results) {
      return finalScreen;
    }
    return confirmationScreen;
  };

  return (
    <Dialog onClose={handleClose} open={open} fullWidth maxWidth="sm">
      {getDialogContent()}
    </Dialog>
  );
}
