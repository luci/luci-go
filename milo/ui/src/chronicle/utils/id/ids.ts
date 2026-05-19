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
  Check,
  CheckEdit,
  CheckResult,
  Stage,
  StageAttempt,
  StageEdit,
  WorkPlan,
} from '@/proto/turboci/graph/ids/v1/identifier.pb';

import { isIdentifier, root } from './wrap';

export enum WorknodeMode {
  StageIsUnknown,
  StageIsWorknode,
  StageNotWorknode,
}

export function checkToken(name: string, tok: string): void {
  if (tok.length === 0) {
    throw new Error(`${name}: zero length`);
  }
  if (tok.includes(':')) {
    throw new Error(`${name}: "${tok}" contains ":"`);
  }
}

export function checkIdx(name: string, idx: number): number {
  if (idx <= 0 || !Number.isInteger(idx)) {
    throw new Error(`${name}: ${idx} must be in [1, max(int32)]`);
  }
  return idx;
}

export function setWorkplanErr<T>(id: T, workPlanID: string): T {
  checkToken('workPlanID', workPlanID);
  const wp = workplan(workPlanID);
  if (isIdentifier(id)) {
    const r = root(id);
    if (r.check) {
      (r.check as unknown as Record<string, unknown>).workPlan = wp;
    } else if (r.stage) {
      (r.stage as unknown as Record<string, unknown>).workPlan = wp;
    }
  } else if (id && typeof id === 'object') {
    (id as unknown as Record<string, unknown>).workPlan = wp;
  }
  return id;
}

export function setWorkplan<T>(id: T, workPlan: string): T {
  return setWorkplanErr(id, workPlan);
}

export function checkErr(id: string): Check {
  checkToken('id', id);
  return { workPlan: undefined, id };
}

export function checkResultErr(
  checkID: string,
  resultIdx: number,
): CheckResult {
  const cid = checkErr(checkID);
  const idx = checkIdx('resultIdx', resultIdx);
  return {
    check: cid,
    idx,
  };
}

export function checkEditErr(checkID: string, ts: string): CheckEdit {
  const cid = checkErr(checkID);
  if (!ts) {
    throw new Error('id.CheckEdit: zero timestamp');
  }
  return {
    check: cid,
    version: ts,
  };
}

export function stageErr(mode: WorknodeMode, stageID: string): Stage {
  checkToken('stageID', stageID);
  let isWorknode: boolean | undefined;
  if (mode === WorknodeMode.StageIsWorknode) {
    isWorknode = true;
  } else if (mode === WorknodeMode.StageNotWorknode) {
    isWorknode = false;
  }
  return { workPlan: undefined, id: stageID, isWorknode };
}

export function stageAttemptErr(
  mode: WorknodeMode,
  stageID: string,
  attemptIdx: number,
): StageAttempt {
  const sid = stageErr(mode, stageID);
  const idx = checkIdx('attemptIdx', attemptIdx);
  return {
    stage: sid,
    idx,
  };
}

export function stageEditErr(
  mode: WorknodeMode,
  stageID: string,
  ts: string,
): StageEdit {
  const sid = stageErr(mode, stageID);
  if (!ts) {
    throw new Error('id.StageEdit: zero timestamp');
  }
  return {
    stage: sid,
    version: ts,
  };
}

export function workplanErr(id: string): WorkPlan {
  checkToken('workPlanID', id);
  return { id };
}

export function workplan(id: string): WorkPlan {
  return workplanErr(id);
}

export function check(id: string): Check {
  return checkErr(id);
}

export function stage(id: string): Stage {
  return stageErr(WorknodeMode.StageNotWorknode, id);
}

export function stageUnknown(id: string): Stage {
  return stageErr(WorknodeMode.StageIsUnknown, id);
}

export function stageWorkNode(id: string): Stage {
  return stageErr(WorknodeMode.StageIsWorknode, id);
}
