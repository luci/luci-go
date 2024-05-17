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

import { OutputTestVariantBranch } from '@/test_verdict/types';

export interface BlamelistState {
  readonly testVariantBranch: OutputTestVariantBranch | null;
  readonly focusCommitPosition: string | null;
}

export type Action = ShowBlamelistAction | DismissAction;

export interface ShowBlamelistAction {
  readonly type: 'showBlamelist';
  readonly testVariantBranch: OutputTestVariantBranch;
  readonly focusCommitPosition: string | null;
}

export interface DismissAction {
  readonly type: 'dismiss';
}

export function reducer(
  _state: BlamelistState,
  action: Action,
): BlamelistState {
  switch (action.type) {
    case 'showBlamelist':
      return {
        testVariantBranch: action.testVariantBranch,
        focusCommitPosition: action.focusCommitPosition,
      };
    case 'dismiss':
      return { testVariantBranch: null, focusCommitPosition: null };
  }
}
