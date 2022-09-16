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

import {
  render,
  screen,
} from '@testing-library/react';

import { identityFunction } from '@/testing_tools/functions';
import { createMockVariantGroups } from '@/testing_tools/mocks/failures_mock';
import { defaultImpactFilter } from '@/tools/failures_tools';

import FailuresTableFilter from './failures_table_filter';

describe('Test FailureTableFilter component', () => {
  it('should display 3 filters.', async () => {
    render(
        <FailuresTableFilter
          failureFilter="All Failures"
          onFailureFilterChanged={identityFunction}
          impactFilter={defaultImpactFilter}
          onImpactFilterChanged={identityFunction}
          variantGroups={createMockVariantGroups()}
          selectedVariantGroups={[]}
          handleVariantGroupsChange={identityFunction}/>,
    );

    await screen.findByTestId('failure_table_filter');

    expect(screen.getByTestId('failure_filter')).toBeInTheDocument();
    expect(screen.getByTestId('impact_filter')).toBeInTheDocument();
    expect(screen.getByTestId('group_by')).toBeInTheDocument();
  });

  it('given non default selected values then should display them', async () => {
    render(
        <FailuresTableFilter
          failureFilter="All Failures"
          onFailureFilterChanged={identityFunction}
          impactFilter={defaultImpactFilter}
          onImpactFilterChanged={identityFunction}
          variantGroups={createMockVariantGroups()}
          selectedVariantGroups={['v1', 'v2']}
          handleVariantGroupsChange={identityFunction}/>,
    );

    await screen.findByTestId('failure_table_filter');

    expect(screen.getByTestId('failure_filter_input')).toHaveValue('All Failures');
    expect(screen.getByTestId('impact_filter_input')).toHaveValue(defaultImpactFilter.name);
    expect(screen.getByTestId('group_by_input')).toHaveValue('v1,v2');
  });
});
