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

import { identityFunction } from '@/clusters/testing_tools/functions';

import ExonerationsTableHead from './exonerations_table_head';

describe('Test ExonerationsTableHead', () => {
  it('should display sortable table head', async () => {
    render(
      <table>
        <ExonerationsTableHead
          isAscending={false}
          toggleSort={identityFunction}
          sortField={'beingExonerated'}
        />
      </table>,
    );

    await screen.findByTestId('exonerations_table_head');

    expect(screen.getByText('Test')).toBeInTheDocument();
    expect(screen.getByText('Variant')).toBeInTheDocument();
    expect(screen.getByText('History')).toBeInTheDocument();
    expect(screen.getByText('Variant')).toBeInTheDocument();
    expect(
      screen.getByText('Presubmit-Blocking Failures Exonerated (7 days)'),
    ).toBeInTheDocument();
    expect(screen.getByText('Last Exoneration')).toBeInTheDocument();
  });
});
