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
/* eslint-disable @typescript-eslint/no-non-null-assertion */

import '@testing-library/jest-dom';
import 'node-fetch';

import { fireEvent, screen, waitFor } from '@testing-library/react';
import fetchMock from 'fetch-mock-jest';

import { noopStateChanger } from '@/clusters/testing_tools/functions';
import { renderWithRouterAndClient } from '@/clusters/testing_tools/libs/mock_router';
import { mockFetchAuthState } from '@/clusters/testing_tools/mocks/authstate_mock';
import { mockFetchProjectConfig } from '@/clusters/testing_tools/mocks/projects_mock';
import {
  createDefaultMockRule,
  mockFetchRule,
  mockUpdateRule,
} from '@/clusters/testing_tools/mocks/rule_mock';
import { Rule } from '@/proto/go.chromium.org/luci/analysis/proto/v1/rules.pb';

import BugEditDialog from './bug_edit_dialog';

describe('Test BugEditDialog component', () => {
  beforeEach(() => {
    mockFetchProjectConfig();
    mockFetchAuthState();
    mockFetchRule(createDefaultMockRule());
  });

  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  it('given a bug, then should display details', async () => {
    renderWithRouterAndClient(
      <BugEditDialog open setOpen={noopStateChanger} />,
      '/p/chromium/rules/1234567',
      '/p/:project/rules/:id',
    );

    await screen.findByText('Save');

    expect(screen.getByText('Save')).toBeInTheDocument();
    expect(screen.getByText('Cancel')).toBeInTheDocument();
    expect(screen.getByText('Bug number')).toBeInTheDocument();
  });

  it('when cancelled, then should revert changes made', async () => {
    renderWithRouterAndClient(
      <BugEditDialog open setOpen={noopStateChanger} />,
      '/p/chromium/rules/1234567',
      '/p/:project/rules/:id',
    );

    await screen.findByText('Save');
    fireEvent.change(screen.getByTestId('bug-number'), {
      target: { value: '6789' },
    });

    expect(screen.getByTestId('bug-number')).toHaveValue('6789');

    fireEvent.click(screen.getByText('Cancel'));

    await waitFor(() =>
      expect(screen.getByTestId('bug-number')).toHaveValue('920702'),
    );
  });

  it('when changing bug details, then should update rule', async () => {
    renderWithRouterAndClient(
      <BugEditDialog open setOpen={noopStateChanger} />,
      '/p/chromium/rules/1234567',
      '/p/:project/rules/:id',
    );

    await screen.findByText('Save');
    fireEvent.change(screen.getByTestId('bug-number'), {
      target: { value: '6789' },
    });

    const updatedRule: Rule = {
      ...createDefaultMockRule(),
      bug: {
        id: '6789',
        linkText: 'new-bug',
        system: 'buganizer',
        url: 'http://linktobug',
      },
    };
    mockUpdateRule(updatedRule);

    fireEvent.click(screen.getByText('Save'));
    await waitFor(
      () =>
        fetchMock.lastCall() !== undefined &&
        fetchMock.lastCall()![0] ===
          'https://staging.analysis.api.luci.app/prpc/luci.analysis.v1.Rules/Update',
    );
    expect(fetchMock.lastCall()![1]!.body).toEqual(
      '{"rule":' +
        '{"name":"projects/chromium/rules/ce83f8395178a0f2edad59fc1a167818",' +
        '"bug":{"system":"buganizer","id":"6789"' +
        '}},' +
        '"updateMask":"bug","etag":"W/\\"2022-01-31T03:36:14.89643Z\\""}',
    );
    expect(screen.getByTestId('bug-number')).toHaveValue('6789');
  });
});
