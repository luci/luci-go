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

import { wrap } from './wrap';

function requireType<T>(msg: unknown, typeName: string): T {
  if (!msg) {
    throw new Error(`missing required "${typeName}"`);
  }
  return msg as T;
}

function requireInt(trimmed: string): number {
  const val = Number(trimmed);
  if (Number.isNaN(val) || !Number.isInteger(val)) {
    throw new Error(`expected a positive integer, got "${trimmed}"`);
  }
  if (val <= 0) {
    throw new Error(`bad idx: must be > 0, got ${val}`);
  }
  return val;
}

function requireTs(trimmed: string): string {
  const parts = trimmed.split('/');
  if (parts.length !== 2) {
    throw new Error('bad timestamp: needed seconds/nanos');
  }
  if (!/^\d+$/.test(parts[0])) {
    throw new Error(`bad timestamp: parsing seconds "${parts[0]}"`);
  }
  if (!/^\d+$/.test(parts[1])) {
    throw new Error(`bad timestamp: parsing nanos "${parts[1]}"`);
  }
  const seconds = Number(parts[0]);
  const nanos = Number(parts[1]);
  if (Number.isNaN(seconds) || Number.isNaN(nanos)) {
    throw new Error(
      `bad timestamp: parsing seconds/nanos "${parts[0]}/${parts[1]}"`,
    );
  }
  const millis = seconds * 1000 + nanos / 1000000;
  return new Date(millis).toISOString();
}

/**
 * Converts a canonical string representation of a TurboCI identifier into its protobuf message form.
 */
export function fromString(id: string): Identifier {
  if (!id) {
    return {};
  }

  const toks = id.split(':');
  let current: unknown = null;

  for (let i = 0; i < toks.length; i++) {
    const tok = toks[i];
    if (tok.length === 0) {
      if (i !== 0) {
        throw new Error(`token ${i} in "${id}" was empty?`);
      }
    } else {
      const key = tok[0];
      const trimmed = tok.substring(1);

      const badKeyErr = () => {
        throw new Error(`unexpected key '${key}'`);
      };

      if (i !== 0 && trimmed.length === 0) {
        throw new Error(`token ${i} in "${id}": unexpected empty token`);
      }

      try {
        switch (key) {
          case 'L':
            if (current !== null) {
              badKeyErr();
            } else if (trimmed.length > 0) {
              current = { id: trimmed } as WorkPlan;
            }
            break;

          case 'C': {
            let wp: WorkPlan | undefined;
            if (current !== null) {
              wp = requireType<WorkPlan>(current, 'WorkPlan');
            }
            current = {
              workPlan: wp,
              id: trimmed,
            } as Check;
            break;
          }

          case 'R': {
            const chk = requireType<Check>(current, 'Check');
            const idx = requireInt(trimmed);
            current = {
              check: chk,
              idx,
            } as CheckResult;
            break;
          }

          case 'V': {
            if (
              current &&
              typeof current === 'object' &&
              'isWorknode' in current
            ) {
              // It's a Stage Edit
              current = {
                stage: current as unknown as Stage,
                version: requireTs(trimmed),
              } as StageEdit;
            } else {
              // It's a Check Edit
              const chk = requireType<Check>(current, 'Check');
              current = {
                check: chk,
                version: requireTs(trimmed),
              } as CheckEdit;
            }
            break;
          }

          case 'N':
          case 'S':
          case '?': {
            let wp: WorkPlan | undefined;
            if (current !== null) {
              wp = requireType<WorkPlan>(current, 'WorkPlan');
            }
            const isWorknode = key === '?' ? undefined : key === 'N';
            current = {
              workPlan: wp,
              id: trimmed,
              isWorknode,
            } as Stage;
            break;
          }

          case 'A': {
            const stg = requireType<Stage>(current, 'Stage');
            const idx = requireInt(trimmed);
            current = {
              stage: stg,
              idx,
            } as StageAttempt;
            break;
          }

          default:
            badKeyErr();
        }
      } catch (err: unknown) {
        throw new Error(`token ${i} in "${id}": ${(err as Error).message}`);
      }
    }
  }

  return wrap(current) || {};
}
