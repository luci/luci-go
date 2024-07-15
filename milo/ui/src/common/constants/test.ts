// Copyright 2023 The LUCI Authors.
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

import { TestStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { TestVariantStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

export const VERDICT_STATUS_DISPLAY_MAP = Object.freeze({
  [TestVariantStatus.EXONERATED]: 'exonerated',
  [TestVariantStatus.EXPECTED]: 'expected',
  [TestVariantStatus.FLAKY]: 'flaky',
  [TestVariantStatus.UNEXPECTED]: 'unexpectedly failed',
  [TestVariantStatus.UNEXPECTEDLY_SKIPPED]: 'unexpectedly skipped',
  [TestVariantStatus.TEST_VARIANT_STATUS_UNSPECIFIED]: 'unspecified',
});

export const TEST_STATUS_DISPLAY_MAP = Object.freeze({
  [TestStatus.STATUS_UNSPECIFIED]: 'unspecified',
  [TestStatus.PASS]: 'passed',
  [TestStatus.FAIL]: 'failed',
  [TestStatus.SKIP]: 'skipped',
  [TestStatus.CRASH]: 'crashed',
  [TestStatus.ABORT]: 'aborted',
});

// This list defines the order in which variant def keys should be displayed.
// Any unrecognized keys will be listed after the ones defined below.
export const ORDERED_VARIANT_DEF_KEYS = Object.freeze([
  'bucket',
  'builder',
  'test_suite',
]);

export const ARTIFACT_LENGTH_LIMIT = 50000;
