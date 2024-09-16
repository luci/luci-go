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

import { OutputTestResultBundle } from '@/test_verdict/types';

/**
 * From the provided list of test results, find the one that should be
 * investigated first and return its index.
 *
 * Currently, this will return the index of the first unexpected result or 0
 * if all results are expected. This may change in the future.
 */
export function getSuggestedResultId(
  results: readonly OutputTestResultBundle[],
) {
  const firstFailedResult = results.find((e) => !e.result?.expected);
  return firstFailedResult
    ? firstFailedResult.result.resultId
    : results[0].result.resultId;
}
