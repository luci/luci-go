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

import { PERM_BUILDS_ADD, PERM_BUILDS_CANCEL } from '@/build/constants';
import { usePermCheck } from '@/common/components/perm_check_provider';
import { Trinary } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

import { useBuild } from '../../context';

export const enum Dialog {
  None,
  CancelBuild,
  RetryBuild,
}

export interface ActionButtonProps {
  openDialog: (dialog: Dialog) => void;
}

export function ActionButton({ openDialog }: ActionButtonProps) {
  const build = useBuild();
  const realm = build && `${build.builder.project}:${build.builder.bucket}`;
  const [canRetry] = usePermCheck(realm, PERM_BUILDS_ADD);
  const [canCancel] = usePermCheck(realm, PERM_BUILDS_CANCEL);

  if (!build) {
    return <></>;
  }

  if (build.endTime) {
    let tooltip = '';
    if (build.retriable === Trinary.NO) {
      tooltip = 'This build cannot be retried.';
    } else if (!canRetry) {
      tooltip = "You don't have the permission to retry this build.";
    }

    return (
      // Use a span to display tooltip with button is disabled.
      <span title={tooltip}>
        <Button
          onClick={() => openDialog(Dialog.RetryBuild)}
          disabled={!canRetry || build.retriable === Trinary.NO}
        >
          Retry Build
        </Button>
      </span>
    );
  }

  let tooltip = '';
  if (build.cancelTime) {
    tooltip = 'The build is already scheduled to be canceled.';
  } else if (!canCancel) {
    tooltip = "You don't have the permission to cancel this build.";
  }

  return (
    // Use a span to display tooltip with button is disabled.
    <span title={tooltip}>
      <Button
        onClick={() => openDialog(Dialog.CancelBuild)}
        disabled={!canCancel || Boolean(build.cancelTime)}
      >
        Cancel Build
      </Button>
    </span>
  );
}
