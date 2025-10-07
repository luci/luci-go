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

import { render, screen, waitFor } from '@testing-library/react';

import { createDefaultMockRule } from '@/clusters/testing_tools/mocks/rule_mock';

import TimestampInfoBar from './timestamp_info_bar';

// Disabled until we can support dayjs relative time format.
describe('Test TimestampInfoBar component', () => {
  it('when proved with rule, then should render username and timestamps', async () => {
    const rule = createDefaultMockRule();
    render(
      <TimestampInfoBar
        createUsername={rule.createUser}
        createTime={rule.createTime}
        updateUsername={rule.lastAuditableUpdateUser}
        updateTime={rule.lastAuditableUpdateTime}
      />,
    );
    await waitFor(() => screen.getByTestId('timestamp-info-bar-create'));

    expect(screen.getByTestId('timestamp-info-bar-create')).toHaveTextContent(
      'Created by LUCI Analysis a few seconds ago',
    );
    expect(screen.getByTestId('timestamp-info-bar-update')).toHaveTextContent(
      'Last modified by user@example.com a few seconds ago',
    );
  });

  it('when provided with a google account, then should display name only', async () => {
    const rule = createDefaultMockRule();
    rule.createUser = 'googler@google.com';
    render(
      <TimestampInfoBar
        createUsername={rule.createUser}
        createTime={rule.createTime}
        updateUsername={rule.lastAuditableUpdateUser}
        updateTime={rule.lastAuditableUpdateTime}
      />,
    );
    await waitFor(() => screen.getByTestId('timestamp-info-bar-create'));

    expect(screen.getByTestId('timestamp-info-bar-create')).toHaveTextContent(
      'Created by googler a few seconds ago',
    );
    expect(screen.getByTestId('timestamp-info-bar-update')).toHaveTextContent(
      'Last modified by user@example.com a few seconds ago',
    );
  });

  it('when provided with an external user, then should use full username', async () => {
    const rule = createDefaultMockRule();
    rule.createUser = 'user@example.com';
    render(
      <TimestampInfoBar
        createUsername={rule.createUser}
        createTime={rule.createTime}
        updateUsername={rule.lastAuditableUpdateUser}
        updateTime={rule.lastAuditableUpdateTime}
      />,
    );
    await waitFor(() => screen.getByTestId('timestamp-info-bar-create'));

    expect(screen.getByTestId('timestamp-info-bar-create')).toHaveTextContent(
      'Created by user@example.com a few seconds ago',
    );
    expect(screen.getByTestId('timestamp-info-bar-update')).toHaveTextContent(
      'Last modified by user@example.com a few seconds ago',
    );
  });
  it('when provided with no user, then only time should be displayed', async () => {
    const rule = createDefaultMockRule();
    rule.createUser = '';
    rule.lastAuditableUpdateUser = '';
    render(
      <TimestampInfoBar
        createUsername={rule.createUser}
        createTime={rule.createTime}
        updateUsername={rule.lastAuditableUpdateUser}
        updateTime={rule.lastAuditableUpdateTime}
      />,
    );
    await waitFor(() => screen.getByTestId('timestamp-info-bar-create'));

    expect(screen.getByTestId('timestamp-info-bar-create')).toHaveTextContent(
      'Created a few seconds ago',
    );
    expect(screen.getByTestId('timestamp-info-bar-update')).toHaveTextContent(
      'Last modified a few seconds ago',
    );
  });
});
