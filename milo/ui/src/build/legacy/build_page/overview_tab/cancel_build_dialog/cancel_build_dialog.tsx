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

import { LoadingButton } from '@mui/lab';
import {
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  TextField,
} from '@mui/material';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';

import { useBuildsClient } from '@/build/hooks/prpc_clients';
import { CancelBuildRequest } from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';

import { useBuild } from '../../context';

export interface CancelBuildDialogProps {
  readonly open: boolean;
  readonly onClose?: () => void;
  readonly container?: HTMLDivElement;
}

export function CancelBuildDialog({
  open,
  onClose,
  container,
}: CancelBuildDialogProps) {
  const [reason, setReason] = useState('');
  const [showError, setShowErr] = useState(false);

  const build = useBuild();
  const queryClient = useQueryClient();
  const client = useBuildsClient();
  const cancelBuildMutation = useMutation({
    mutationFn: (req: CancelBuildRequest) => client.CancelBuild(req),
    onSuccess: () => {
      // TODO(b/335064206): invalidate the cancelled build only.
      queryClient.invalidateQueries();
      onClose?.();
    },
    // TODO(b/335064206): handle failure.
  });

  if (!build) {
    return <></>;
  }

  const handleConfirm = () => {
    if (!reason) {
      setShowErr(true);
      return;
    }
    cancelBuildMutation.mutate(
      CancelBuildRequest.fromPartial({
        id: build.id,
        summaryMarkdown: reason,
      }),
    );
  };
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
        <LoadingButton
          onClick={handleConfirm}
          variant="contained"
          loading={cancelBuildMutation.isLoading}
        >
          Confirm
        </LoadingButton>
      </DialogActions>
    </Dialog>
  );
}
