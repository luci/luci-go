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

import Mustache from 'mustache';
import { RenderOptions } from 'mustache';

import { StringPair } from '@/common/services/common';
import { logging } from '@/common/tools/logging';
import { renderMarkdown } from '@/common/tools/markdown/utils';
import {
  Instruction,
  InstructionTarget,
  TargetedInstruction,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/instruction.pb';

/**
 * targetedInstructionMap returns a mapping between target and targeted instruction.
 * The map returned will be in ordered: LOCAL, REMOTE, PREBUILT.
 */
export function targetedInstructionMap(
  instruction: Instruction | undefined,
): Map<InstructionTarget, TargetedInstruction> {
  const map = new Map<InstructionTarget, TargetedInstruction>();
  if (instruction === undefined) {
    return map;
  }

  // The uniqueness of the target is guaranteed.
  for (const targetedInstruction of instruction.targetedInstructions) {
    for (const target of targetedInstruction.targets) {
      map.set(target, targetedInstruction);
    }
  }
  const sortedMap = new Map(
    [...map.entries()].sort(([target1], [target2]) => target1 - target2),
  );
  return sortedMap;
}

export function pairsToPlaceholderDict(
  data: readonly StringPair[] | undefined,
): {
  [key: string]: unknown;
} {
  const result: { [key: string]: string } = {};
  // We do not support repeated keys for now.
  // So later value of the same will override the previous one.
  for (const pair of data || []) {
    result[pair.key] = pair.value || '';
  }
  return result;
}

export function renderMustacheMarkdown(
  content: string | undefined,
  placeholderData: object,
): string {
  if (content === undefined) {
    return '';
  }
  // Mustache.render replaces placeholder with empty string '' if
  // the value is not found. But we handle the error just in case
  // so a bad template does not break the whole page.
  try {
    // This is logic-less markdown. We don't want to escape value
    // inside placeholders, leave them as-is. The HTML will be
    // sanitized before rendering.
    const customEscape = (value: string) => value;
    const options: RenderOptions = { escape: customEscape };
    const markDownContent = Mustache.render(
      content,
      placeholderData,
      undefined,
      options,
    );
    return renderMarkdown(markDownContent || '');
  } catch (error) {
    logging.warn('error rendering markdown', error);
    return '';
  }
}

/**
 * A quick way to know if we need to load dependency build from buildbucket.
 * We check for the present of the {{build...}} tag
 */
export function shouldLoadDependencyBuild(
  content: string | undefined,
): boolean {
  if (content === undefined) {
    return false;
  }
  return /{{build\..*}}/.test(content);
}
