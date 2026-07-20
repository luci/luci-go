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
  Identifier,
  Stage,
  StageAttempt,
  StageEdit,
  WorkPlan,
} from '@/proto/turboci/graph/ids/v1/identifier.pb';
import { IdentifierKind } from '@/proto/turboci/graph/ids/v1/identifier_kind.pb';

import { kindOf, wrap } from './wrap';

export const INVALID_IDENTIFIER = '<turboci invalid identifier>';

/**
 * Converts any TurboCI identifier proto into its canonical string format.
 */
export function toString(id: unknown): string {
  if (!id) {
    return INVALID_IDENTIFIER;
  }

  let current: Identifier | null = wrap(id);
  if (!current) {
    return INVALID_IDENTIFIER;
  }

  const fmtVersion = (versionStr: string | undefined): string => {
    if (!versionStr) return '0/0';
    const date = new Date(versionStr);
    if (isNaN(date.getTime())) {
      // If version is already structured in seconds/nanos or otherwise, handle it gracefully
      const parts = versionStr.split('/');
      if (parts.length === 2) {
        return versionStr;
      }
      return '0/0';
    }
    const seconds = Math.trunc(date.getTime() / 1000);
    const nanos = (date.getTime() % 1000) * 1000000;
    return `${seconds}/${nanos}`;
  };

  const acc: string[] = [];
  let hasWorkPlan = false;

  while (current) {
    const kind = kindOf(current);
    switch (kind) {
      case IdentifierKind.IDENTIFIER_KIND_WORK_PLAN: {
        const x: WorkPlan | undefined = current.workPlan;
        if (x?.id) {
          acc.push('L' + x.id);
          hasWorkPlan = true;
        }
        current = null;
        break;
      }
      case IdentifierKind.IDENTIFIER_KIND_CHECK: {
        const x: Check | undefined = current.check;
        if (!x) {
          current = null;
          break;
        }
        acc.push('C' + (x.id || ''));
        current = x.workPlan ? { workPlan: x.workPlan } : null;
        break;
      }
      case IdentifierKind.IDENTIFIER_KIND_CHECK_RESULT: {
        const x: CheckResult | undefined = current.checkResult;
        if (!x) {
          current = null;
          break;
        }
        acc.push('R' + String(x.idx));
        current = x.check ? { check: x.check } : null;
        break;
      }
      case IdentifierKind.IDENTIFIER_KIND_CHECK_EDIT: {
        const x: CheckEdit | undefined = current.checkEdit;
        if (!x) {
          current = null;
          break;
        }
        acc.push('V' + fmtVersion(x.version));
        current = x.check ? { check: x.check } : null;
        break;
      }
      case IdentifierKind.IDENTIFIER_KIND_STAGE: {
        const x: Stage | undefined = current.stage;
        if (!x) {
          current = null;
          break;
        }
        let prefix = '?';
        if (x.isWorknode !== undefined) {
          prefix = x.isWorknode ? 'N' : 'S';
        }
        acc.push(prefix + (x.id || ''));
        current = x.workPlan ? { workPlan: x.workPlan } : null;
        break;
      }
      case IdentifierKind.IDENTIFIER_KIND_STAGE_ATTEMPT: {
        const x: StageAttempt | undefined = current.stageAttempt;
        if (!x) {
          current = null;
          break;
        }
        acc.push('A' + String(x.idx));
        current = x.stage ? { stage: x.stage } : null;
        break;
      }
      case IdentifierKind.IDENTIFIER_KIND_STAGE_EDIT: {
        const x: StageEdit | undefined = current.stageEdit;
        if (!x) {
          current = null;
          break;
        }
        acc.push('V' + fmtVersion(x.version));
        current = x.stage ? { stage: x.stage } : null;
        break;
      }
      default:
        current = null;
        break;
    }
  }

  if (acc.length > 0 && !hasWorkPlan) {
    acc.push('');
  }

  acc.reverse();
  return acc.join(':');
}
