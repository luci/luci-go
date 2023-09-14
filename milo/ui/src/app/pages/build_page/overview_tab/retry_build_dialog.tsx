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
  DialogContentText,
  DialogTitle,
} from '@mui/material';
import { observer } from 'mobx-react-lite';
import { useCallback } from 'react';
import { useNavigate } from 'react-router-dom';

import { useStore } from '@/common/store';
import { getBuildURLPathFromBuildData } from '@/common/tools/url_utils';

export interface RetryBuildDialogProps {
  readonly open: boolean;
  readonly onClose?: () => void;
  readonly container?: HTMLDivElement;
}

export const RetryBuildDialog = observer(
  ({ open, onClose, container }: RetryBuildDialogProps) => {
    const pageState = useStore().buildPage;
    const navigate = useNavigate();

    const handleConfirm = useCallback(async () => {
      const build = await pageState.retryBuild();
      onClose?.();
      if (build) {
        navigate(getBuildURLPathFromBuildData(build));
      }
    }, [pageState]);

    return (
      <Dialog
        onClose={onClose}
        open={open}
        fullWidth
        maxWidth="sm"
        container={container}
      >
        <DialogTitle>Retry Build</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Note: this doesnt trigger anything else (e.g. CQ).
          </DialogContentText>
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
