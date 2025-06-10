// Copyright 2022 The LUCI Authors.
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

import { ExoneratedTestVariantBuilder } from '../../model/mocks';
import { ChromiumCriteria } from '../../model/model';

import FailureCriteriaSection from './failure_criteria_section';

describe('Test FailureCriteriaSection', () => {
  it('shows statistics related to the criteria', async () => {
    const testVariant = new ExoneratedTestVariantBuilder().build();
    render(
      <FailureCriteriaSection
        testVariant={testVariant}
        criteria={ChromiumCriteria}
      />,
    );

    expect(screen.getByTestId('unexpected_verdict_count')).toHaveTextContent(
      '(current value: 2)',
    );

    // Check examples appear in the page.
    expect(screen.getByText('333333333333')).toBeInTheDocument();
    expect(screen.getByText('changelist-three #8333')).toBeInTheDocument();
    expect(screen.getByText('No failing run')).toBeInTheDocument();
    expect(screen.getByText('444444444444')).toBeInTheDocument();
    expect(screen.getByText('Has failing run')).toBeInTheDocument();
  });
});
