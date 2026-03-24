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

import { ContentCopy as ContentCopyIcon } from '@mui/icons-material';
import {
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  IconButton,
  InputAdornment,
  Switch,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material';
import { useEffect, useState } from 'react';

import { DashboardState } from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

export interface ShareDashboardDialogProps {
  /**
   * Whether the modal is open.
   */
  open: boolean;
  /**
   * Callback for when the modal is closed.
   */
  onClose: () => void;
  /**
   * The current dashboard state.
   */
  dashboardState: DashboardState | null;
  /**
   * Callback when permissions are applied.
   */
  onApplyPermissions: (isPublic: boolean) => void;
  /**
   * Whether the request is currently saving.
   */
  isPending?: boolean;
}

/**
 * A dialog that allows users to copy a dashboard link and toggle public access.
 */
export function ShareDashboardDialog({
  open,
  onClose,
  dashboardState,
  onApplyPermissions,
  isPending = false,
}: ShareDashboardDialogProps) {
  const [copied, setCopied] = useState(false);
  const currentUrl = window.location.href;

  const isPublicStored = dashboardState?.isPublic ?? false;

  const [draftIsPublic, setDraftIsPublic] = useState(isPublicStored);

  // Reset the draft state whenever the dialog opens or the stored state changes
  useEffect(() => {
    if (open) {
      setDraftIsPublic(isPublicStored);
    }
  }, [open, isPublicStored]);

  useEffect(() => {
    if (!copied) return;
    const timer = setTimeout(() => setCopied(false), 2000);
    return () => clearTimeout(timer);
  }, [copied]);

  const handleCopy = () => {
    navigator.clipboard.writeText(currentUrl);
    setCopied(true);
  };

  const handleToggle = (event: React.ChangeEvent<HTMLInputElement>) => {
    setDraftIsPublic(event.target.checked);
  };

  const handleApplyPermissions = () => {
    onApplyPermissions(draftIsPublic);
  };

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="xs">
      <DialogTitle>Share Dashboard</DialogTitle>
      <DialogContent>
        <Box
          sx={{
            display: 'flex',
            flexDirection: 'column',
            gap: 2,
            pt: 1,
          }}
        >
          <Box>
            <Typography
              variant="caption"
              color="text.secondary"
              display="block"
              gutterBottom
            >
              Link to this view
            </Typography>
            <TextField
              fullWidth
              size="small"
              value={currentUrl}
              variant="outlined"
              InputProps={{
                readOnly: true,
                endAdornment: (
                  <InputAdornment position="end">
                    <Tooltip title={copied ? 'Copied!' : 'Copy to clipboard'}>
                      <IconButton onClick={handleCopy} edge="end" size="small">
                        <ContentCopyIcon fontSize="small" />
                      </IconButton>
                    </Tooltip>
                  </InputAdornment>
                ),
              }}
            />
          </Box>

          <Box sx={{ pt: 1, borderTop: '1px solid', borderColor: 'divider' }}>
            <FormControlLabel
              control={
                <Switch
                  checked={draftIsPublic}
                  onChange={handleToggle}
                  color="primary"
                  size="small"
                />
              }
              label={
                <Box>
                  <Typography variant="body2">Public Access</Typography>
                  <Typography variant="caption" color="text.secondary">
                    Anyone with the link can view
                  </Typography>
                </Box>
              }
            />
          </Box>
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={isPending}>
          Cancel
        </Button>
        <Button
          onClick={handleApplyPermissions}
          variant="contained"
          disabled={draftIsPublic === isPublicStored || isPending}
        >
          {isPending ? 'Saving...' : 'Apply Changes'}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
