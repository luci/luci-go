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

import { DateTime } from 'luxon';

import { isWorknodeStage } from '@/chronicle/utils/check_utils';
import { extractLegacyWorkNode } from '@/chronicle/utils/legacy_worknode';
import { parseTimestamp } from '@/chronicle/utils/time_utils';
import {
  Stage,
  Stage_Attempt,
  Stage_Attempt_Progress,
} from '@/proto/turboci/graph/orchestrator/v1/stage.pb';
import { StageAttemptState } from '@/proto/turboci/graph/orchestrator/v1/stage_attempt_state.pb';
import { ValueData } from '@/proto/turboci/graph/orchestrator/v1/value_data.pb';

/** Read-only view of the resolved inline values for a work plan. */
export type ValueDataMap =
  | Map<string, ValueData>
  | ReadonlyMap<string, ValueData>;

/**
 * One contiguous slice of a stage's timeline bar, derived from a single
 * progress message.
 */
export interface ProgressSegment {
  /** Inclusive start of the slice, clamped to the stage's bounds. */
  readonly start: DateTime;
  /** Exclusive end of the slice, clamped to the stage's bounds. */
  readonly end: DateTime;
  /** Short human-readable text drawn inside the segment when wide enough. */
  readonly label: string;
  /** The unmodified progress message, shown in the tooltip. */
  readonly rawMessage: string;
  /** 1-based attempt number that produced this segment. */
  readonly attemptNumber: number;
}

/**
 * Intermediate timestamped progress message parsed from an attempt,
 * used during segment generation to construct chronological time slices.
 */
interface ProgressPoint {
  readonly ms: number;
  readonly message: string;
}

const BUILD_PROGRESS_PREFIX = /^Build \S+ for (?:work )?node \S+\s+(?:has\s+)?/;
const TYPE_URL_ATP_MESSAGE =
  'type.googleapis.com/wireless.android.launchcontrol.WorkNode.ProgressMessage.AtpMachineReadableMessage';

function getProgressPrefix(
  stage: Stage,
  valueDataMap: ValueDataMap,
): RegExp | undefined {
  if (!isWorknodeStage(stage)) {
    return undefined;
  }
  const workNode = extractLegacyWorkNode(stage, valueDataMap);
  const type = (workNode?.workExecutorType ?? '').trim().toUpperCase();
  if (type === 'PENDING_CHANGE_BUILD' || type === 'SUBMITTED_BUILD') {
    return BUILD_PROGRESS_PREFIX;
  }
  return undefined;
}

function formatProgressLabel(message: string, prefix?: RegExp): string {
  const text = message.trim();
  return prefix ? text.replace(prefix, '') : text;
}

function extractProgressMessage(
  progress: Stage_Attempt_Progress,
  valueDataMap: ValueDataMap,
): string | undefined {
  // Standard stages populate human-readable text directly in progress.message.
  if (progress.message?.trim()) {
    return progress.message;
  }

  // ATP test stages leave progress.message empty and instead attach an
  // AtpMachineReadableMessage in progress.details.
  const atpDetail = progress.details.find(
    (d) => d.typeUrl === TYPE_URL_ATP_MESSAGE,
  );
  const json = atpDetail?.digest
    ? valueDataMap.get(atpDetail.digest)?.json?.value
    : undefined;
  if (!json) {
    return undefined;
  }

  try {
    const { state } = JSON.parse(json) as { state?: string };
    // COMPLETED is a point-in-time completion marker (like AttemptEnded),
    // not an ongoing phase of work to display as a duration.
    if (!state || state.toUpperCase() === 'COMPLETED') {
      return undefined;
    }
    // Normalize states like "QUEUED" or "RUNNING" to lowercase segment labels.
    return state.toLowerCase();
  } catch {
    return undefined;
  }
}

function parseAttemptProgressPoints(
  attempt: Stage_Attempt,
  valueDataMap: ValueDataMap,
): ProgressPoint[] {
  const points: ProgressPoint[] = [];
  for (const progress of attempt.progress) {
    // Extract message text or ATP state, ignoring empty updates and completion markers.
    const message = extractProgressMessage(progress, valueDataMap);
    if (!message) {
      continue;
    }

    const dt = parseTimestamp(progress.version?.ts);
    if (dt) {
      points.push({ ms: dt.toMillis(), message });
    }
  }

  // Ensure points are strictly chronological before constructing duration segments.
  return points.sort((a, b) => a.ms - b.ms);
}

function attemptStartMs(attempt: Stage_Attempt): number | undefined {
  const first = attempt.stateHistory[0] ?? attempt;
  return parseTimestamp(first.version?.ts)?.toMillis();
}

function isTerminalAttemptState(state?: StageAttemptState): boolean {
  return (
    state === StageAttemptState.STAGE_ATTEMPT_STATE_COMPLETE ||
    state === StageAttemptState.STAGE_ATTEMPT_STATE_INCOMPLETE
  );
}

function attemptEndMs(attempt: Stage_Attempt, stageEndMs: number): number {
  const last = attempt.stateHistory.at(-1) ?? attempt;
  // If the attempt reached a terminal state (COMPLETE or INCOMPLETE), its
  // execution ended at the timestamp of that final state transition.
  if (isTerminalAttemptState(last.state)) {
    const ts = parseTimestamp(last.version?.ts)?.toMillis();
    if (ts !== undefined) {
      return ts;
    }
  }

  // Fallback to the parent stage's end timestamp (when the stage reached FINAL)
  // if the attempt is missing a terminal state transition.
  return stageEndMs;
}

/**
 * Converts a stage's attempt progress messages into chronological timeline
 * segments bounded by each attempt's execution window. Time gaps between
 * attempts remain empty.
 *
 * @param stage The stage containing attempts and progress messages.
 * @param valueDataMap Resolved inline work plan values for executor type checks.
 * @param stageEnd The end timestamp bounding the stage row.
 * @returns An array of chronological progress segments across all attempts.
 */
export function buildProgressSegments(
  stage: Stage,
  valueDataMap: ValueDataMap,
  stageEnd: DateTime,
): ProgressSegment[] {
  const attempts = stage.attempts ?? [];
  if (attempts.length === 0) {
    return [];
  }

  const prefix = getProgressPrefix(stage, valueDataMap);
  const stageEndMs = stageEnd.toMillis();
  const allSegments: ProgressSegment[] = [];

  for (let idx = 0; idx < attempts.length; idx++) {
    const attempt = attempts[idx];
    const attemptNumber = idx + 1;
    const points = parseAttemptProgressPoints(attempt, valueDataMap);
    if (points.length === 0) {
      continue;
    }

    const startMs = attemptStartMs(attempt) ?? points[0].ms;
    const endMs = attemptEndMs(attempt, stageEndMs);

    for (let i = 0; i < points.length; i++) {
      // The first segment starts at the attempt's start time and the last
      // segment ends at the attempt's end time. Every other segment in
      // between starts and ends at their own progress update timestamp.
      const from = i === 0 ? Math.min(startMs, points[0].ms) : points[i].ms;
      const to = i + 1 < points.length ? points[i + 1].ms : endMs;
      if (to <= from) {
        continue;
      }

      const label = formatProgressLabel(points[i].message, prefix);
      const prev = allSegments.at(-1);
      // Collapse consecutive segments within the same attempt that have
      // identical labels into a single continuous segment.
      if (
        prev &&
        prev.attemptNumber === attemptNumber &&
        prev.label.toLowerCase() === label.toLowerCase()
      ) {
        allSegments[allSegments.length - 1] = {
          ...prev,
          end: DateTime.fromMillis(to),
        };
      } else {
        allSegments.push({
          start: DateTime.fromMillis(from),
          end: DateTime.fromMillis(to),
          label,
          rawMessage: points[i].message,
          attemptNumber,
        });
      }
    }
  }

  return allSegments;
}
