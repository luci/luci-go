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

import { Button, ButtonTypeMap } from '@mui/material';
import { DefaultComponentProps } from '@mui/material/OverridableComponent';
import { useState } from 'react';

import { PERM_BUILDS_CANCEL } from '@/build/constants';
import { OutputBuild } from '@/build/types';
import { usePermCheck } from '@/common/components/perm_check_provider';

import { CancelBuildDialog } from './cancel_build_dialog';

export interface CancelBuildButtonProps
  extends Omit<DefaultComponentProps<ButtonTypeMap>, 'onClick' | 'disabled'> {
  readonly build: OutputBuild;
}

export function CancelBuildButton({ build, ...props }: CancelBuildButtonProps) {
  const [openDialog, setOpenDialog] = useState(false);
  const realm = `${build.builder.project}:${build.builder.bucket}`;
  const [canCancel] = usePermCheck(realm, PERM_BUILDS_CANCEL);

  let tooltip = '';
  if (build.cancelTime) {
    tooltip = 'The build is already scheduled to be canceled.';
  } else if (!canCancel) {
    tooltip = "You don't have the permission to cancel this build.";
  }

  return (
    <>
      <CancelBuildDialog
        buildId={build.id}
        open={openDialog}
        onClose={() => setOpenDialog(false)}
      />
      {/* Use a span so the tooltip works even when the button is disabled. */}
      <span title={tooltip}>
        <Button
          {...props}
          onClick={() => setOpenDialog(true)}
          disabled={!canCancel || Boolean(build.cancelTime)}
        >
          Cancel Build
        </Button>
      </span>
    </>
  );
}
