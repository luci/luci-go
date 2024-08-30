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

import { reducer } from './reducer';

describe('reducer', () => {
  it('can update commit range', () => {
    const prevState = {
      commitPositionRange: {
        last: '200',
        first: '100',
      },
      testVariantBranch: null,
      focusCommitPosition: '',
    };
    const nextState = reducer(prevState, {
      type: 'setBlamelistRange',
      lastCommitPosition: '400',
      firstCommitPosition: '100',
    });
    expect(nextState).not.toBe(prevState);
    expect(nextState).toEqual({
      commitPositionRange: {
        last: '400',
        first: '100',
      },
      testVariantBranch: null,
      focusCommitPosition: '',
    });
  });

  it('should not update state when commit range is unchanged', () => {
    const prevState = {
      commitPositionRange: {
        last: '200',
        first: '100',
      },
      testVariantBranch: null,
      focusCommitPosition: '',
    };
    const nextState = reducer(prevState, {
      type: 'setBlamelistRange',
      lastCommitPosition: '200',
      firstCommitPosition: '100',
    });
    expect(nextState).toBe(prevState);
    expect(nextState).toEqual({
      commitPositionRange: {
        last: '200',
        first: '100',
      },
      testVariantBranch: null,
      focusCommitPosition: '',
    });
  });
});
