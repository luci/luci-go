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

import '@testing-library/jest-dom';

import { render, screen } from '@testing-library/react';

import {
  ExoneratedTestVariantBranchBuilder,
  TestCriteria,
} from '../../model/mocks';

import FailureCriteriaSection from './failure_criteria_section';

describe('Test FailureCriteriaSection', () => {
  it('shows statistics related to the criteria', async () => {
    const testVariantBranch = new ExoneratedTestVariantBranchBuilder().build();
    testVariantBranch.failureRate.unexpectedTestRuns = 101;
    testVariantBranch.failureRate.consecutiveUnexpectedTestRuns = 102;

    render(
      <FailureCriteriaSection
        testVariantBranch={testVariantBranch}
        criteria={TestCriteria}
      />,
    );

    expect(screen.getByTestId('unexpected_verdict_count')).toHaveTextContent(
      '(current value: 101)',
    );
    expect(
      screen.getByTestId('consecutive_unexpected_verdict_count'),
    ).toHaveTextContent('(current value: 102)');

    // Check examples appear in the page.
    expect(screen.getByText('333333333333')).toBeInTheDocument();
    expect(screen.getByText('changelist-three #8333')).toBeInTheDocument();
    expect(screen.getByText('1/1^')).toBeInTheDocument();
    expect(screen.getByText('444444444444')).toBeInTheDocument();
    expect(screen.getByText('1/9*')).toBeInTheDocument();
  });
});
