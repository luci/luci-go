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

import { ReactNode } from 'react';

import { TestStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import {
  TestVerdict_Status,
  TestVerdict_StatusOverride,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';
import { Duration } from '@/proto/google/protobuf/duration.pb';
import { OutputTestVerdict } from '@/test_verdict/types';

import { TestVerdictProvider } from '../context';

export const placeholderVerdict = TestVariant.fromPartial({
  testId: 'tast.inputs.VirtualKeyboardAutocorrect.fr_fr_a11y',
  variant: {
    def: {
      build_target: 'betty-pi-arc',
    },
  },
  variantHash: '6657d9dc1549eacc',
  statusV2: TestVerdict_Status.FLAKY,
  statusOverride: TestVerdict_StatusOverride.NOT_OVERRIDDEN,
  results: [
    {
      result: {
        testId: 'test1234',
        name:
          'invocations/u-chrome-bot-2023-10-25-09-08-00-26592efa1f477db0/tests/tast.inputs' +
          '.VirtualKeyboardAutocorrect.fr_fr_a11y/results/87ecc8c3-00063',
        resultId: '87ecc8c3-00063',
        status: TestStatus.FAIL,
        summaryHtml: '<text-artifact artifact-id="Test Log" />',
        startTime: '2023-10-25T09:01:00.167244802Z',
        duration: Duration.fromPartial({
          seconds: '55.567',
        }),
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
            'Failed to validate VK autocorrect: failed to validate VK autocorrect on step 4:' +
            ' failed to validate field text on step 2: failed to validate input value: got:' +
            ' francais ; want: français',
        },
      },
    },
  ],
  testMetadata: {
    name: 'tast.inputs.VirtualKeyboardAutocorrect.fr_fr_a11y',
    location: {
      fileName: 'test_file.test',
      repo: 'chromium.googlesource.com',
    },
  },
  sourcesId: '1cd6a26e26fdac2078adcb8c',
}) as OutputTestVerdict;

interface Props {
  readonly children: ReactNode;
}

export function FakeTestVerdictContextProvider({ children }: Props) {
  return (
    <TestVerdictProvider
      invocationID="inv-123456"
      project="chromium"
      testVerdict={placeholderVerdict}
      sources={null}
    >
      {children}
    </TestVerdictProvider>
  );
}
