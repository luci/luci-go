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

import { TestResult_Status } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

export function normalizeFailureReason(message?: string): string {
  if (!message || message.trim() === '') {
    return 'No failure reason string specified.';
  }
  let normalized = message;
  const clusterRegex =
    /[/+0-9a-zA-Z]{10,}=+|[-0-9a-fA-F\s\t]{16,}|(?:0x)?[0-9a-fA-F]{8,}|[0-9]+/g;
  normalized = normalized.replace(clusterRegex, '?');
  return normalized.trim() || 'No failure reason string specified.';
}

export function normalizeDrawerFailureReason(message?: string): string {
  if (!message || message.trim() === '') {
    return '';
  }
  return normalizeFailureReason(message);
}

export function getTestDisplayName(tv: TestVariant): string {
  if (tv.testMetadata?.name && tv.testMetadata.name !== tv.testId) {
    return tv.testMetadata.name;
  }
  return tv.testId;
}

export function getProjectFromRealm(realm?: string): string | undefined {
  if (!realm) {
    return undefined;
  }
  const parts = realm.split(':');
  return parts[0];
}

export function getStatusColor(status?: TestResult_Status): string {
  switch (status) {
    case TestResult_Status.FAILED:
      return 'error.main';
    case TestResult_Status.PASSED:
      return 'success.main';
    default:
      return 'text.secondary';
  }
}
