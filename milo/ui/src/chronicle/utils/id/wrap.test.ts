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

import { IdentifierKind } from '@/proto/turboci/graph/ids/v1/identifier_kind.pb';

import {
  check,
  checkEditErr,
  setWorkplan,
  stage,
  stageAttemptErr,
  workplan,
} from './ids';
import { kindOf, root, sameRoot, sameWorkPlan, unwrap, wrap } from './wrap';

describe('chronicle/utils/id/wrap', () => {
  it('kindOf returns correct type kinds', () => {
    const wp = workplan('wp');
    expect(kindOf(wrap(wp))).toEqual(IdentifierKind.IDENTIFIER_KIND_WORK_PLAN);

    const chk = setWorkplan(check('chk'), 'wp');
    expect(kindOf(wrap(chk))).toEqual(IdentifierKind.IDENTIFIER_KIND_CHECK);

    const st = setWorkplan(stage('st'), 'wp');
    expect(kindOf(wrap(st))).toEqual(IdentifierKind.IDENTIFIER_KIND_STAGE);
  });

  it('sameRoot resolves roots correctly matching Go behavior', () => {
    const c1 = check('c');
    const c2 = checkEditErr('c', '2026-05-19T12:00:00.000Z');
    const c3 = check('other');

    expect(sameRoot(c1, c2)).toBe(true);
    expect(sameRoot(c1, c3)).toBe(false);
    expect(sameRoot(c1, null)).toBe(false);

    const s1 = stage('s');
    // stageAttemptErr with StageNotWorknode (2)
    const sa = stageAttemptErr(2, 's', 1);
    const s1wp = setWorkplan(stage('s'), 'wp');

    expect(sameRoot(c1, s1)).toBe(false);
    expect(sameRoot(s1, s1wp)).toBe(false); // should be false because workplans differ
    expect(sameRoot(s1, sa)).toBe(true);
  });

  it('sameWorkPlan resolves workplans correctly matching Go behavior', () => {
    const c1 = setWorkplan(check('c1'), 'wp');
    const c2 = setWorkplan(check('c2'), 'wp');
    const c3 = setWorkplan(check('c1'), 'other');

    expect(sameWorkPlan(c1, c2)).toBe(true);
    expect(sameWorkPlan(c1, c3)).toBe(false);
    expect(sameWorkPlan(c1, null)).toBe(false);

    // Two identifiers with no workplan should have the same (null) workplan
    const s1 = stage('s');
    const chk = check('c');
    expect(sameWorkPlan(s1, chk)).toBe(true);
  });

  it('unwraps properly', () => {
    const chk = check('chk');
    const wrapped = wrap(chk);
    expect(unwrap(wrapped)).toEqual(chk);
  });

  it('returns correct roots', () => {
    const wp = workplan('wp');
    const chk = setWorkplan(check('chk'), 'wp');

    const res = root(chk);
    expect(res.wp).toEqual(wp);
    expect(res.check).toEqual(chk);
    expect(res.stage).toBeNull();
  });
});
