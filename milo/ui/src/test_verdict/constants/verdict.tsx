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

import {
  TestVerdict_Status,
  TestVerdict_StatusOverride,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';

export const VERDICT_STATUS_COLOR_MAP = Object.freeze({
  [TestVerdict_Status.FAILED]: 'var(--failure-color)',
  [TestVerdict_Status.EXECUTION_ERRORED]: 'var(--critical-failure-color)',
  [TestVerdict_Status.PRECLUDED]: 'var(--precluded-color)',
  [TestVerdict_Status.FLAKY]: 'var(--warning-color)',
  [TestVerdict_Status.PASSED]: 'var(--success-color)',
  [TestVerdict_Status.SKIPPED]: 'var(--skipped-color)',
});

export const VERDICT_STATUS_OVERRIDE_COLOR_MAP = Object.freeze({
  [TestVerdict_StatusOverride.EXONERATED]: 'var(--exonerated-color)',
});
