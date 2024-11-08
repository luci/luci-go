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
/* eslint-disable @typescript-eslint/no-empty-function */

import '@testing-library/jest-dom';
import 'node-fetch';

import { screen } from '@testing-library/react';
import fetchMock from 'fetch-mock-jest';
import { DateTime } from 'luxon';

import { renderWithRouterAndClient } from '@/clusters/testing_tools/libs/mock_router';
import { mockFetchAuthState } from '@/clusters/testing_tools/mocks/authstate_mock';
import { mockReclusteringProgress } from '@/clusters/testing_tools/mocks/cluster_mock';
import {
  createMockDoneProgress,
  createMockProgress,
} from '@/clusters/testing_tools/mocks/progress_mock';

import ReclusteringProgressIndicator from './reclustering_progress_indicator';

describe('Test ReclusteringProgressIndicator component', () => {
  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  it('given an finished progress, then should not display', async () => {
    mockFetchAuthState();
    mockReclusteringProgress(createMockDoneProgress());
    renderWithRouterAndClient(
      <ReclusteringProgressIndicator
        project="chromium"
        hasRule
        rulePredicateLastUpdated={DateTime.now()
          .minus({
            minute: 5,
          })
          .toISO()}
      />,
    );

    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('given a progress, then should display percentage', async () => {
    mockFetchAuthState();
    mockReclusteringProgress(createMockProgress(800));
    renderWithRouterAndClient(
      <ReclusteringProgressIndicator
        project="chromium"
        hasRule
        rulePredicateLastUpdated={DateTime.now()
          .minus({
            minute: 5,
          })
          .toISO()}
      />,
    );

    await screen.findByRole('alert');
    await screen.findByText('80%');

    expect(screen.getByText('80%')).toBeInTheDocument();
  });

  // Disabled as it flakes.
  // eslint-disable-next-line jest/no-disabled-tests
  it.skip('when progress is done, then should display button to refresh analysis', async () => {
    mockFetchAuthState();
    mockReclusteringProgress(createMockProgress(800));
    renderWithRouterAndClient(
      <ReclusteringProgressIndicator
        project="chromium"
        hasRule
        rulePredicateLastUpdated={DateTime.now()
          .minus({
            minute: 5,
          })
          .toISO()}
      />,
    );
    await screen.findByRole('alert');
    await screen.findByText('80%');

    mockReclusteringProgress(createMockDoneProgress());

    await screen.findByRole('button');
    expect(screen.getByText('View updated impact')).toBeInTheDocument();
  });
});
