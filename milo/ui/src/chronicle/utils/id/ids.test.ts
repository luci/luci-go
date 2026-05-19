// Copyright 2026 The LUCI Authors.
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
  check,
  checkErr,
  checkResultErr,
  setWorkplan,
  stage,
  stageUnknown,
  stageWorkNode,
} from './ids';

describe('chronicle/utils/id/ids', () => {
  it('creates check identifiers', () => {
    expect(check('my-check')).toEqual({ id: 'my-check' });
    expect(() => checkErr('bad:id')).toThrow('id: "bad:id" contains ":"');
    expect(() => checkErr('')).toThrow('id: zero length');
  });

  it('creates checkResultErr correctly', () => {
    const res = checkResultErr('my-check', 2);
    expect(res).toEqual({
      check: { id: 'my-check' },
      idx: 2,
    });
    expect(() => checkResultErr('my-check', -1)).toThrow(
      'resultIdx: -1 must be in [1, max(int32)]',
    );
    expect(() => checkResultErr('my-check', 0)).toThrow(
      'resultIdx: 0 must be in [1, max(int32)]',
    );
  });

  it('creates stage identifiers with proper modes', () => {
    expect(stage('st')).toEqual({ id: 'st', isWorknode: false });
    expect(stageUnknown('st')).toEqual({ id: 'st', isWorknode: undefined });
    expect(stageWorkNode('st')).toEqual({ id: 'st', isWorknode: true });
  });

  it('sets workplan on identifiers', () => {
    const chk = check('my-check');
    const updated = setWorkplan(chk, 'wp');
    expect(updated.workPlan).toEqual({ id: 'wp' });
  });
});
