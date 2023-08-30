// Copyright 2023 The LUCI Authors.
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
import { observer } from 'mobx-react-lite';

import { useStore } from '@/common/store';

export const enum Dialog {
  None,
  CancelBuild,
  RetryBuild,
}

export interface ActionsSectionProps {
  openDialog: (dialog: Dialog) => void;
}

export const ActionsSection = observer(
  ({ openDialog }: ActionsSectionProps) => {
    const store = useStore();
    const build = store.buildPage.build;

    if (!build) {
      return <></>;
    }

    const canRetry = store.buildPage.canRetry;

    if (build.endTime) {
      return (
        <>
          <h3>Actions</h3>
          <div
            title={
              canRetry ? '' : 'You have no permission to retry this build.'
            }
          >
            <Button
              onClick={() => openDialog(Dialog.RetryBuild)}
              disabled={!canRetry}
            >
              Retry Build
            </Button>
          </div>
        </>
      );
    }

    const canCancel = build.cancelTime === null && store.buildPage.canCancel;
    let tooltip = '';
    if (!canCancel) {
      tooltip =
        build.cancelTime === null
          ? 'You have no permission to cancel this build.'
          : 'The build is already scheduled to be canceled.';
    }

    return (
      <>
        <h3>Actions</h3>
        <div title={tooltip}>
          <Button
            onClick={() => openDialog(Dialog.CancelBuild)}
            disabled={!canCancel}
          >
            Cancel Build
          </Button>
        </div>
      </>
    );
  },
);
