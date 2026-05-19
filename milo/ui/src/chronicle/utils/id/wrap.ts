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

export type ConcreteIdentifier =
  | WorkPlan
  | Check
  | CheckResult
  | CheckEdit
  | Stage
  | StageAttempt
  | StageEdit;

/**
 * Type guard to identify if an object is an Identifier container.
 */
export function isIdentifier(obj: unknown): obj is Identifier {
  if (!obj || typeof obj !== 'object') return false;
  const record = obj as Record<string, unknown>;
  if ('id' in record || 'idx' in record || 'version' in record) return false;
  return (
    'workPlan' in record ||
    'check' in record ||
    'checkResult' in record ||
    'checkEdit' in record ||
    'stage' in record ||
    'stageAttempt' in record ||
    'stageEdit' in record
  );
}

export function isWorkPlan(obj: unknown): obj is WorkPlan {
  if (!obj || typeof obj !== 'object') return false;
  const record = obj as Record<string, unknown>;
  return (
    'id' in record &&
    !(
      'workPlan' in record ||
      'idx' in record ||
      'version' in record ||
      'isWorknode' in record
    )
  );
}

export function isCheck(obj: unknown): obj is Check {
  if (!obj || typeof obj !== 'object') return false;
  const record = obj as Record<string, unknown>;
  return (
    'id' in record &&
    'workPlan' in record &&
    !('idx' in record || 'version' in record || 'isWorknode' in record)
  );
}

export function isStage(obj: unknown): obj is Stage {
  if (!obj || typeof obj !== 'object') return false;
  const record = obj as Record<string, unknown>;
  return (
    'id' in record &&
    'workPlan' in record &&
    'isWorknode' in record &&
    !('idx' in record || 'version' in record)
  );
}

export function isCheckResult(obj: unknown): obj is CheckResult {
  if (!obj || typeof obj !== 'object') return false;
  const record = obj as Record<string, unknown>;
  return 'idx' in record && 'check' in record;
}

export function isStageAttempt(obj: unknown): obj is StageAttempt {
  if (!obj || typeof obj !== 'object') return false;
  const record = obj as Record<string, unknown>;
  return 'idx' in record && 'stage' in record;
}

export function isCheckEdit(obj: unknown): obj is CheckEdit {
  if (!obj || typeof obj !== 'object') return false;
  const record = obj as Record<string, unknown>;
  return 'version' in record && 'check' in record;
}

export function isStageEdit(obj: unknown): obj is StageEdit {
  if (!obj || typeof obj !== 'object') return false;
  const record = obj as Record<string, unknown>;
  return 'version' in record && 'stage' in record;
}

export function isConcreteIdentifier(obj: unknown): obj is ConcreteIdentifier {
  return (
    isWorkPlan(obj) ||
    isCheck(obj) ||
    isCheckResult(obj) ||
    isCheckEdit(obj) ||
    isStage(obj) ||
    isStageAttempt(obj) ||
    isStageEdit(obj)
  );
}

/**
 * Wraps any specific Identifier sub-type into a generic Identifier container.
 */
export function wrap(id: unknown): Identifier | null {
  if (!id) return null;
  if (isIdentifier(id)) return id;

  if (isWorkPlan(id)) return { workPlan: id };
  if (isCheck(id)) return { check: id };
  if (isCheckResult(id)) return { checkResult: id };
  if (isCheckEdit(id)) return { checkEdit: id };
  if (isStage(id)) return { stage: id };
  if (isStageAttempt(id)) return { stageAttempt: id };
  if (isStageEdit(id)) return { stageEdit: id };

  return null;
}

/**
 * Unwraps a generic Identifier container into its concrete underlying type.
 */
export function unwrap(id: unknown): ConcreteIdentifier | null {
  if (!id) return null;
  if (isIdentifier(id)) {
    if (id.workPlan !== undefined) return id.workPlan;
    if (id.check !== undefined) return id.check;
    if (id.checkResult !== undefined) return id.checkResult;
    if (id.checkEdit !== undefined) return id.checkEdit;
    if (id.stage !== undefined) return id.stage;
    if (id.stageAttempt !== undefined) return id.stageAttempt;
    if (id.stageEdit !== undefined) return id.stageEdit;
    return null;
  }
  if (isConcreteIdentifier(id)) {
    return id;
  }
  return null;
}

/**
 * Resolves the stage root of a StageIdentifier.
 */
export function stageRoot(id: unknown): Stage | null {
  const r = root(id);
  return r.stage;
}

/**
 * Resolves the check root of a CheckIdentifier.
 */
export function checkRoot(id: unknown): Check | null {
  const r = root(id);
  return r.check;
}

/**
 * Resolves the base check/stage and workplan associated with any ID container or concrete ID object.
 */
export function root(id: unknown): {
  wp: WorkPlan | null;
  check: Check | null;
  stage: Stage | null;
} {
  let wp: WorkPlan | null = null;
  let check: Check | null = null;
  let stage: Stage | null = null;

  if (isIdentifier(id)) {
    if (id.workPlan !== undefined) {
      wp = id.workPlan;
    } else if (id.check !== undefined) {
      check = id.check;
    } else if (id.checkResult !== undefined) {
      check = id.checkResult.check || null;
    } else if (id.checkEdit !== undefined) {
      check = id.checkEdit.check || null;
    } else if (id.stage !== undefined) {
      stage = id.stage;
    } else if (id.stageAttempt !== undefined) {
      stage = id.stageAttempt.stage || null;
    } else if (id.stageEdit !== undefined) {
      stage = id.stageEdit.stage || null;
    }
  } else {
    const unwrapped = unwrap(id);
    if (!unwrapped) {
      return { wp, check, stage };
    }

    if (isWorkPlan(unwrapped)) {
      wp = unwrapped;
    } else if (isCheck(unwrapped)) {
      check = unwrapped;
    } else if (isStage(unwrapped)) {
      stage = unwrapped;
    } else if (isCheckResult(unwrapped)) {
      check = unwrapped.check || null;
    } else if (isStageAttempt(unwrapped)) {
      stage = unwrapped.stage || null;
    } else if (isCheckEdit(unwrapped)) {
      check = unwrapped.check || null;
    } else if (isStageEdit(unwrapped)) {
      stage = unwrapped.stage || null;
    }
  }

  // Fill in WorkPlan from check or stage if possible
  if (stage) {
    wp = stage.workPlan || null;
  } else if (check) {
    wp = check.workPlan || null;
  }

  return { wp, check, stage };
}

/**
 * Describes any type of Identifier as an IdentifierKind enum.
 */
export function kindOf(id: unknown): IdentifierKind {
  if (isIdentifier(id)) {
    if (id.workPlan !== undefined)
      return IdentifierKind.IDENTIFIER_KIND_WORK_PLAN;
    if (id.check !== undefined) return IdentifierKind.IDENTIFIER_KIND_CHECK;
    if (id.checkResult !== undefined)
      return IdentifierKind.IDENTIFIER_KIND_CHECK_RESULT;
    if (id.checkEdit !== undefined)
      return IdentifierKind.IDENTIFIER_KIND_CHECK_EDIT;
    if (id.stage !== undefined) return IdentifierKind.IDENTIFIER_KIND_STAGE;
    if (id.stageAttempt !== undefined)
      return IdentifierKind.IDENTIFIER_KIND_STAGE_ATTEMPT;
    if (id.stageEdit !== undefined)
      return IdentifierKind.IDENTIFIER_KIND_STAGE_EDIT;
  }

  const unwrapped = unwrap(id);
  if (!unwrapped) {
    throw new Error(`unknown or invalid identifier: ${id}`);
  }

  if (isWorkPlan(unwrapped)) return IdentifierKind.IDENTIFIER_KIND_WORK_PLAN;
  if (isCheck(unwrapped)) return IdentifierKind.IDENTIFIER_KIND_CHECK;
  if (isCheckResult(unwrapped))
    return IdentifierKind.IDENTIFIER_KIND_CHECK_RESULT;
  if (isCheckEdit(unwrapped)) return IdentifierKind.IDENTIFIER_KIND_CHECK_EDIT;
  if (isStage(unwrapped)) return IdentifierKind.IDENTIFIER_KIND_STAGE;
  if (isStageAttempt(unwrapped))
    return IdentifierKind.IDENTIFIER_KIND_STAGE_ATTEMPT;
  if (isStageEdit(unwrapped)) return IdentifierKind.IDENTIFIER_KIND_STAGE_EDIT;

  throw new Error(`impossible type for kindOf: ${JSON.stringify(unwrapped)}`);
}

/**
 * Returns true if the two identifiers have the same root.
 */
export function sameRoot(a: unknown, b: unknown): boolean {
  if (!a || !b) {
    return false;
  }
  const aRoot = root(a);
  const bRoot = root(b);

  if (aRoot.stage && bRoot.stage) {
    return (
      aRoot.stage.id === bRoot.stage.id &&
      aRoot.stage.isWorknode === bRoot.stage.isWorknode &&
      aRoot.stage.workPlan?.id === bRoot.stage.workPlan?.id
    );
  }

  if (!aRoot.stage && !bRoot.stage) {
    if (!aRoot.check && !bRoot.check) {
      return true;
    }
    if (aRoot.check && bRoot.check) {
      return (
        aRoot.check.id === bRoot.check.id &&
        aRoot.check.workPlan?.id === bRoot.check.workPlan?.id
      );
    }
  }

  return false;
}

/**
 * Returns true if the two identifiers have the same workplan.
 */
export function sameWorkPlan(a: unknown, b: unknown): boolean {
  if (!a || !b) {
    return false;
  }
  const aRoot = root(a);
  const bRoot = root(b);

  if (!aRoot.wp && !bRoot.wp) {
    return true;
  }
  if (aRoot.wp && bRoot.wp) {
    return aRoot.wp.id === bRoot.wp.id;
  }
  return false;
}
