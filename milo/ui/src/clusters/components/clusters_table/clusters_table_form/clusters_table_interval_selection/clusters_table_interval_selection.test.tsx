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

import '@testing-library/jest-dom';

import { fireEvent, screen } from '@testing-library/react';

import { renderWithRouter } from '@/clusters/testing_tools/libs/mock_router';

import { ClustersTableIntervalSelection } from './clusters_table_interval_selection';
import { TIME_INTERVAL_OPTIONS } from './constants';

describe('Test ClustersTableIntervalSelection component', () => {
  it('should display the time interval selector', async () => {
    renderWithRouter(<ClustersTableIntervalSelection />);

    await screen.findByTestId('interval-selection');

    await fireEvent.mouseDown(screen.getByRole('combobox'));
    TIME_INTERVAL_OPTIONS.forEach((interval) => {
      expect(screen.getByText(interval.label)).toBeInTheDocument();
    });
  });

  it('given a time interval param in the URL, the interval selection should match', async () => {
    const lastOption = TIME_INTERVAL_OPTIONS[TIME_INTERVAL_OPTIONS.length - 1];
    renderWithRouter(
      <ClustersTableIntervalSelection />,
      `/?interval=${lastOption.id}`,
    );

    await screen.findByTestId('interval-selection');

    expect(screen.getByTestId('clusters-table-interval-selection')).toHaveValue(
      lastOption.id,
    );
  });

  it('should have no selection when given an unrecognized time interval ID', async () => {
    renderWithRouter(<ClustersTableIntervalSelection />, '/?interval=5d');

    await screen.findByTestId('interval-selection');

    expect(screen.getByTestId('clusters-table-interval-selection')).toHaveValue(
      '',
    );
  });
});
