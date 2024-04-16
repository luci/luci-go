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
  DialogContentText,
  DialogTitle,
} from '@mui/material';
import { useMutation } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';

import { useBuildsClient } from '@/build/hooks/prpc_clients';
import { OutputBuild } from '@/build/types';
import { getBuildURLPathFromBuildData } from '@/common/tools/url_utils';
import { ScheduleBuildRequest } from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';

import { useBuild } from '../context';

export interface RetryBuildDialogProps {
  readonly open: boolean;
  readonly onClose?: () => void;
  readonly container?: HTMLDivElement;
}

export function RetryBuildDialog({
  open,
  onClose,
  container,
}: RetryBuildDialogProps) {
  const navigate = useNavigate();
  const build = useBuild();

  const client = useBuildsClient();
  const retryBuildMutation = useMutation({
    mutationFn: (buildId: string) =>
      client.ScheduleBuild(
        ScheduleBuildRequest.fromPartial({ templateBuildId: buildId }),
      ),
    onSuccess: (data) =>
      navigate(getBuildURLPathFromBuildData(data as OutputBuild)),
    // TODO(b/335064206): handle failure.
  });

  if (!build) {
    return <></>;
  }

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
        <LoadingButton
          onClick={() => retryBuildMutation.mutate(build.id)}
          variant="contained"
          loading={retryBuildMutation.isLoading}
        >
          Confirm
        </LoadingButton>
      </DialogActions>
    </Dialog>
  );
}
