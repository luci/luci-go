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
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
} from '@mui/material';

export interface AdminAccessRequiredDialogProps {
  open: boolean;
  onClose: () => void;
}

export function AdminAccessRequiredDialog({
  open,
  onClose,
}: AdminAccessRequiredDialogProps) {
  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>Admin Access Required</DialogTitle>
      <DialogContent>
        <p>
          The action you are trying to perform requires additional permissions.
          To run Admin Tasks from the Fleet Console, please request membership
          in the following group:
        </p>
        <p>
          <a
            href="https://ganpati2.corp.google.com/group/fleet-console-admin-tasks-policy.prod"
            target="_blank"
            rel="noreferrer"
          >
            fleet-console-admin-tasks-policy
          </a>
        </p>
        <p>
          If you think this is a bug, please file it to{' '}
          <a href="http://go/fcon-bug" target="_blank" rel="noreferrer">
            Fleet Console UI
          </a>
          .
        </p>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} variant="contained">
          Close
        </Button>
      </DialogActions>
    </Dialog>
  );
}
