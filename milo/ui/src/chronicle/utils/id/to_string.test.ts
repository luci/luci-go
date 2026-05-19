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
  checkEditErr,
  checkResultErr,
  setWorkplan,
  stage,
  stageAttemptErr,
  stageEditErr,
  stageUnknown,
  stageWorkNode,
  WorknodeMode,
  workplan,
} from './ids';
import { toString } from './to_string';

describe('chronicle/utils/id/to_string', () => {
  it('formats workplan ID', () => {
    expect(toString(workplan('my-workplan'))).toEqual('Lmy-workplan');
  });

  it('formats check on workplan ID', () => {
    const chk = setWorkplan(check('my-check'), 'my-workplan');
    expect(toString(chk)).toEqual('Lmy-workplan:Cmy-check');
  });

  it('formats check result ID', () => {
    expect(toString(checkResultErr('my-check', 2))).toEqual(':Cmy-check:R2');
  });

  it('formats check edit ID', () => {
    const ts = new Date(1715678901234).toISOString();
    expect(toString(checkEditErr('my-check', ts))).toEqual(
      ':Cmy-check:V1715678901/234000000',
    );
  });

  it('formats stage (default/unknown/worknode) IDs', () => {
    const st = setWorkplan(stage('st'), 'wp');
    expect(toString(st)).toEqual('Lwp:Sst');

    const unk = setWorkplan(stageUnknown('st'), 'wp');
    expect(toString(unk)).toEqual('Lwp:?st');

    const wn = setWorkplan(stageWorkNode('st'), 'wp');
    expect(toString(wn)).toEqual('Lwp:Nst');
  });

  it('formats stage attempt IDs', () => {
    expect(
      toString(stageAttemptErr(WorknodeMode.StageNotWorknode, 'st', 123)),
    ).toEqual(':Sst:A123');
  });

  it('formats stage edit IDs', () => {
    const ts = new Date(1715678901234).toISOString();
    expect(
      toString(stageEditErr(WorknodeMode.StageNotWorknode, 'st', ts)),
    ).toEqual(':Sst:V1715678901/234000000');
  });
});
