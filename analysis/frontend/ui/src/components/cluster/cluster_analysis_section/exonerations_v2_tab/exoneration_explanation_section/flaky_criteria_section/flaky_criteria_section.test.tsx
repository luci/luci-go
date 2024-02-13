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

import {
  render,
  screen,
} from '@testing-library/react';

import { ExoneratedTestVariantBranchBuilder, TestCriteria } from '../../model/mocks';
import FlakyCriteriaSection from './flaky_criteria_section';

describe('Test FlakyCriteriaSection', () => {
  it('shows statistics related to the criteria', async () => {
    const testVariantBranch = new ExoneratedTestVariantBranchBuilder().build();
    testVariantBranch.flakeRate.runFlakyVerdicts = 101;
    testVariantBranch.flakeRate.totalVerdicts = 102;

    render(
        <FlakyCriteriaSection
          testVariantBranch={testVariantBranch}
          criteria={TestCriteria}/>,
    );

    expect(screen.getByTestId('flaky_verdicts_7d')).toHaveTextContent('(current value: 101)');
    expect(screen.getByTestId('flaky_verdicts_rate_7d')).toHaveTextContent('(current value: 101 / 102)');

    // Check examples appear in the page.
    expect(screen.getByText('111111111111')).toBeInTheDocument();
    expect(screen.getByText('changelist-one #8111')).toBeInTheDocument();
    expect(screen.getByText('222222222222')).toBeInTheDocument();
  });
});
