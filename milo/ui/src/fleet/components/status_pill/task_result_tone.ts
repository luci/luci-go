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

import { StatusTone } from '@/fleet/theme/tokens';

/**
 * Task result or state (Swarming, Buildbucket, Mobile Harness) -> pill tone and
 * sentence-case label. Exact matches only, so new states fall back to neutral
 * instead of being misread by a substring match.
 */
const TASK_RESULT_TONES: Record<string, { tone: StatusTone; label: string }> = {
  SUCCESS: { tone: 'success', label: 'Passed' },
  PASS: { tone: 'success', label: 'Passed' },
  PASSED: { tone: 'success', label: 'Passed' },
  COMPLETED: { tone: 'success', label: 'Completed' },
  FAILURE: { tone: 'error', label: 'Failure' },
  FAIL: { tone: 'error', label: 'Failure' },
  FAILED: { tone: 'error', label: 'Failure' },
  ERROR: { tone: 'error', label: 'Error' },
  ALLOC_ERROR: { tone: 'error', label: 'Allocation error' },
  ALLOC_FAIL: { tone: 'error', label: 'Allocation error' },
  CLIENT_ERROR: { tone: 'error', label: 'Client error' },
  RUNNING: { tone: 'info', label: 'Running' },
  STARTED: { tone: 'info', label: 'Running' },
  PENDING: { tone: 'info', label: 'Pending' },
  SCHEDULED: { tone: 'info', label: 'Pending' },
  BOT_DIED: { tone: 'warning', label: 'Bot died' },
  TIMED_OUT: { tone: 'warning', label: 'Timed out' },
  TIMEOUT: { tone: 'warning', label: 'Timed out' },
  EXPIRED: { tone: 'warning', label: 'Expired' },
  NO_RESOURCE: { tone: 'warning', label: 'No resource' },
  INFRA_ERROR: { tone: 'warning', label: 'Infra error' },
  INFRA_FAILURE: { tone: 'warning', label: 'Infra failure' },
  KILLED: { tone: 'neutral', label: 'Killed' },
  CANCELED: { tone: 'neutral', label: 'Canceled' },
  CANCELLED: { tone: 'neutral', label: 'Canceled' },
  SKIPPED: { tone: 'neutral', label: 'Skipped' },
};

function toSentenceCase(raw: string): string {
  const words = raw.replace(/_/g, ' ').toLowerCase();
  return words.charAt(0).toUpperCase() + words.slice(1);
}

export function taskResultTone(raw: string): {
  tone: StatusTone;
  label: string;
} {
  return (
    TASK_RESULT_TONES[raw.toUpperCase()] ?? {
      tone: 'neutral',
      label: toSentenceCase(raw),
    }
  );
}
