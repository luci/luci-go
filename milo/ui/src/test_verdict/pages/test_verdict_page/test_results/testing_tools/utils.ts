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
  TestResult,
  TestStatus,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';

export function createFakeTestResult(resultName: string): TestResult {
  return TestResult.fromPartial({
    testId: 'tast.inputs.VirtualKeyboardAutocorrect.fr_fr_a11y',
    name: resultName,
    resultId: '87ecc8c3-00063',
    status: TestStatus.FAIL,
    summaryHtml: '<text-artifact artifact-id="Test Log" />',
    startTime: '2023-10-25T09:01:00.167244802Z',
    duration: {
      seconds: '55',
      nanos: 567000000,
    },
    tags: [
      {
        key: 'ancestor_buildbucket_ids',
        value: '8766287273535464561',
      },
      {
        key: 'board',
        value: 'betty-pi-arc',
      },
      {
        key: 'bug_component',
        value: 'b:95887',
      },
    ],
    failureReason: {
      primaryErrorMessage:
        'Failed to validate VK autocorrect: failed to validate VK autocorrect on step 4: failed' +
        ' to validate field text on step 2: failed to validate input value: got: francais ;' +
        ' want: français',
    },
  });
}
