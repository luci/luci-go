// Copyright 2022 The LUCI Authors.
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
  TextField,
} from '@mui/material';
import { observer } from 'mobx-react-lite';
import { useCallback, useState } from 'react';

import { useStore } from '@/common/store';

export interface CancelBuildDialogProps {
  readonly open: boolean;
  readonly onClose?: () => void;
  readonly container?: HTMLDivElement;
}

export const CancelBuildDialog = observer(
  ({ open, onClose, container }: CancelBuildDialogProps) => {
    const pageState = useStore().buildPage;
    const [reason, setReason] = useState('');
    const [showError, setShowErr] = useState(false);

    const handleConfirm = useCallback(() => {
      if (!reason) {
        setShowErr(true);
        return;
      }
      pageState.cancelBuild(reason);
      onClose?.();
    }, [reason, pageState, onClose]);

    const handleUpdate = (newReason: string) => {
      setReason(newReason);
      setShowErr(false);
    };

    return (
      <Dialog
        onClose={onClose}
        open={open}
        fullWidth
        maxWidth="sm"
        container={container}
      >
        <DialogTitle>Cancel Build</DialogTitle>
        <DialogContent>
          <TextField
            label="Reason"
            value={reason}
            error={showError}
            helperText={showError ? 'Reason is required' : ''}
            onChange={(e) => handleUpdate(e.target.value)}
            required
            margin="dense"
            fullWidth
            multiline
            minRows={4}
            maxRows={10}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={onClose} variant="text">
            Dismiss
          </Button>
          <Button onClick={handleConfirm} variant="contained">
            Confirm
          </Button>
        </DialogActions>
      </Dialog>
    );
  },
);
