// Copyright 2025 The LUCI Authors.
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

import { Chip } from '@mui/material';

import {
  getStatusStyle,
  SemanticStatusType,
} from '@/common/styles/status_styles';
import {
  FailureReason_Kind,
  failureReason_KindToJSON,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/failure_reason.pb';
import {
  SkippedReason_Kind,
  TestResult_Status,
  skippedReason_KindToJSON,
  testResult_StatusToJSON,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';

interface StatusKindChipProps {
  statusV2?: TestResult_Status;
  failureKindKeyPart?: FailureReason_Kind;
  skippedKindKeyPart?: SkippedReason_Kind;
}

function getResultStatusV2DisplayText(statusV2?: TestResult_Status): string {
  if (statusV2 === undefined) return 'N/A';
  switch (statusV2) {
    case TestResult_Status.PASSED:
      return 'Passed';
    case TestResult_Status.FAILED:
      return 'Failed';
    case TestResult_Status.SKIPPED:
      return 'Skipped';
    case TestResult_Status.EXECUTION_ERRORED:
      return 'Execution Error';
    case TestResult_Status.PRECLUDED:
      return 'Precluded';
    case TestResult_Status.STATUS_UNSPECIFIED:
      return 'Unspecified';
    default:
      return testResult_StatusToJSON(statusV2) || 'Unknown';
  }
}

function getSemanticStatus(statusV2?: TestResult_Status): SemanticStatusType {
  if (
    statusV2 === undefined ||
    statusV2 === TestResult_Status.STATUS_UNSPECIFIED
  ) {
    return 'unknown';
  }
  return testResult_StatusToJSON(statusV2).toLowerCase() as SemanticStatusType;
}

function getFailureReasonKindDisplayText(
  kind?: FailureReason_Kind,
): string | undefined {
  if (
    kind === undefined ||
    kind === FailureReason_Kind.KIND_UNSPECIFIED ||
    kind === FailureReason_Kind.ORDINARY
  ) {
    return undefined;
  }
  return failureReason_KindToJSON(kind)
    .split('_')
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1).toLowerCase())
    .join(' ');
}

function getSkippedReasonKindDisplayText(
  kind?: SkippedReason_Kind,
): string | undefined {
  if (kind === undefined || kind === SkippedReason_Kind.KIND_UNSPECIFIED) {
    return undefined;
  }
  return skippedReason_KindToJSON(kind)
    .split('_')
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1).toLowerCase())
    .join(' ');
}

export function StatusKindChip({
  statusV2,
  failureKindKeyPart,
  skippedKindKeyPart,
}: StatusKindChipProps) {
  const statusDisplay = getResultStatusV2DisplayText(statusV2);
  const failureKindDisplay =
    getFailureReasonKindDisplayText(failureKindKeyPart);
  const skippedKindDisplay =
    getSkippedReasonKindDisplayText(skippedKindKeyPart);

  let combinedLabel = statusDisplay;
  if (failureKindDisplay) {
    combinedLabel = `${statusDisplay} - ${failureKindDisplay}`;
  } else if (skippedKindDisplay) {
    combinedLabel = `${statusDisplay} - ${skippedKindDisplay}`;
  }

  const semanticStatus = getSemanticStatus(statusV2);
  const style = getStatusStyle(semanticStatus);

  return (
    <Chip
      size="small"
      label={combinedLabel}
      sx={{ backgroundColor: style.backgroundColor, color: style.textColor }}
    />
  );
}
