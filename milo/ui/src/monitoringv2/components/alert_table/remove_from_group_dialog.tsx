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

import {
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
} from '@mui/material';

import { StructuredAlert } from '@/monitoringv2/util/alerts';
import { AlertGroup } from '@/proto/go.chromium.org/luci/luci_notify/api/service/v1/alert_groups.pb';

interface RemoveFromGroupDialogProps {
  onClose: () => void;
  group: AlertGroup;
  alerts: StructuredAlert[];
  onConfirm: () => void;
}

export const RemoveFromGroupDialog = ({
  onClose,
  alerts,
  group,
  onConfirm: removeFromGroup,
}: RemoveFromGroupDialogProps) => {
  return (
    <Dialog open onClose={onClose}>
      <DialogTitle>Remove alert{alerts.length > 1 ? 's' : ''}</DialogTitle>
      <DialogContent>
        Are you sure you want to remove{' '}
        {alerts.length > 1 ? `these ${alerts.length}` : 'this'} alert
        {alerts.length > 1 ? 's' : ''} from the group{' '}
        <strong>{group.displayName}</strong>?
      </DialogContent>
      <DialogActions>
        <Button onClick={() => onClose()}>Cancel</Button>
        <Button
          onClick={() => {
            removeFromGroup();
            onClose();
          }}
        >
          Remove Alerts
        </Button>
      </DialogActions>
    </Dialog>
  );
};
