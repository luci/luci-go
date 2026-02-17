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
  AggregationLevel,
  TestIdentifier,
  TestIdentifierPrefix,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

export function getTestIdentifierPrefixId(
  prefix: TestIdentifierPrefix,
): string {
  const p = prefix.id;
  if (!p) return `LEVEL:${prefix.level}`;

  const parts = [`LEVEL:${prefix.level}`];
  if (p.moduleName) parts.push(`MOD:${p.moduleName}`);
  if (p.moduleScheme) parts.push(`SCH:${p.moduleScheme}`);

  if (p.moduleVariantHash) {
    parts.push(`VAR:${p.moduleVariantHash}`);
  }
  if (p.coarseName) parts.push(`CRS:${p.coarseName}`);
  if (p.fineName) parts.push(`FNE:${p.fineName}`);
  if (p.caseName) parts.push(`CAS:${p.caseName}`);

  return parts.join('|');
}

export function createPrefixForTest(
  testVariant: { testIdStructured?: TestVariant['testIdStructured'] },
  level: AggregationLevel,
): TestIdentifierPrefix {
  const struct = testVariant.testIdStructured;
  if (!struct) {
    return { level, id: TestIdentifier.fromPartial({}) };
  }

  return {
    level,
    id: TestIdentifier.fromPartial({
      moduleName: struct.moduleName,
      moduleScheme: struct.moduleScheme,
      moduleVariant: struct.moduleVariant,
      moduleVariantHash: struct.moduleVariantHash,
      coarseName:
        level === AggregationLevel.COARSE ||
        level === AggregationLevel.FINE ||
        level === AggregationLevel.CASE
          ? struct.coarseName
          : undefined,
      fineName:
        level === AggregationLevel.FINE || level === AggregationLevel.CASE
          ? struct.fineName
          : undefined,
      caseName: level === AggregationLevel.CASE ? struct.caseName : undefined,
    }),
  };
}
