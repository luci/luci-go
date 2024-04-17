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

import { Button } from '@mui/material';
import { useState } from 'react';

import { PERM_BUILDS_ADD } from '@/build/constants';
import { OutputBuild } from '@/build/types';
import { usePermCheck } from '@/common/components/perm_check_provider';
import { Trinary } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

import { RetryBuildDialog } from './retry_build_dialog';

export const enum Dialog {
  None,
  CancelBuild,
  RetryBuild,
}

export interface RetryBuildButtonProps {
  readonly build: OutputBuild;
}

export function RetryBuildButton({ build }: RetryBuildButtonProps) {
  const [openDialog, setOpenDialog] = useState(false);
  const realm = `${build.builder.project}:${build.builder.bucket}`;
  const [canRetry] = usePermCheck(realm, PERM_BUILDS_ADD);

  let tooltip = '';
  if (build.retriable === Trinary.NO) {
    tooltip = 'This build cannot be retried.';
  } else if (!canRetry) {
    tooltip = "You don't have the permission to retry this build.";
  }

  return (
    <>
      <RetryBuildDialog
        buildId={build.id}
        open={openDialog}
        onClose={() => setOpenDialog(false)}
      />
      {/* Use a span to display tooltip with button is disabled. */}
      <span title={tooltip}>
        <Button
          onClick={() => setOpenDialog(true)}
          disabled={!canRetry || build.retriable === Trinary.NO}
        >
          Retry Build
        </Button>
      </span>
    </>
  );
}
