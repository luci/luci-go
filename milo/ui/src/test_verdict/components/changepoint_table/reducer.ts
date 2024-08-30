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

import { OutputTestVariantBranch } from '@/analysis/types';

export interface BlamelistState {
  readonly commitPositionRange: {
    readonly last: string;
    readonly first: string;
  } | null;
  readonly testVariantBranch: OutputTestVariantBranch | null;
  readonly focusCommitPosition: string | null;
}

export type Action =
  | SetBlamelistRangeAction
  | ShowBlamelistAction
  | DismissAction;

export interface SetBlamelistRangeAction {
  readonly type: 'setBlamelistRange';
  readonly lastCommitPosition: string;
  readonly firstCommitPosition: string;
}

export interface ShowBlamelistAction {
  readonly type: 'showBlamelist';
  readonly testVariantBranch: OutputTestVariantBranch;
  readonly focusCommitPosition: string | null;
}

export interface DismissAction {
  readonly type: 'dismiss';
}

export function reducer(state: BlamelistState, action: Action): BlamelistState {
  switch (action.type) {
    case 'setBlamelistRange': {
      const unchanged =
        state.commitPositionRange?.last === action.lastCommitPosition &&
        state.commitPositionRange?.first === action.firstCommitPosition;
      if (unchanged) {
        return state;
      }
      return {
        ...state,
        commitPositionRange: {
          last: action.lastCommitPosition,
          first: action.firstCommitPosition,
        },
      };
    }
    case 'showBlamelist':
      return {
        ...state,
        testVariantBranch: action.testVariantBranch,
        focusCommitPosition: action.focusCommitPosition,
      };
    case 'dismiss':
      return { ...state, testVariantBranch: null, focusCommitPosition: null };
  }
}
