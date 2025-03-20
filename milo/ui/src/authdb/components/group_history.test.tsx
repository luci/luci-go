// Copyright 2025 The LUCI Authors.
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

import List from '@mui/material/List';
import { render, screen } from '@testing-library/react';
import { DateTime } from 'luxon';

import { stripPrefix } from '@/authdb/common/helpers';
import { GroupHistory } from '@/authdb/components/group_history';
import {
  createMockGroupHistory,
  mockFetchGetHistory,
} from '@/authdb/testing_tools/mocks/group_history_mock';
import { LONG_TIME_FORMAT } from '@/common/tools/time_utils';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

describe('<GroupHistory />', () => {
  test('displays group changes: changetype, who, when, details', async () => {
    const mockHistory = createMockGroupHistory('123');
    mockFetchGetHistory(mockHistory);

    render(
      <FakeContextProvider>
        <List>
          <GroupHistory name="123" />
        </List>
      </FakeContextProvider>,
    );

    await screen.findByTestId('history-table');
    for (const change of mockHistory.changes) {
      expect(screen.getByText(change.changeType)).toBeInTheDocument();
      expect(
        screen.getByText(stripPrefix('user', change.who)),
      ).toBeInTheDocument();
      const dateString = DateTime.fromISO(change.when || '').toFormat(
        LONG_TIME_FORMAT,
      );
      expect(screen.getByText(dateString)).toBeInTheDocument();
    }
  });
  test('displays group change details', async () => {
    const mockHistory = createMockGroupHistory('123');
    mockFetchGetHistory(mockHistory);

    render(
      <FakeContextProvider>
        <List>
          <GroupHistory name="123" />
        </List>
      </FakeContextProvider>,
    );

    await screen.findByTestId('history-table');
    // Shows group member added changes.
    let change = mockHistory.changes[0];
    for (const member of change.members) {
      const memberString = '+ ' + stripPrefix('user', member);
      expect(screen.getByText(memberString)).toBeInTheDocument();
    }
    // Shows group glob removed changes.
    change = mockHistory.changes[1];
    for (const glob of change.globs) {
      const globString = '- ' + glob;
      expect(screen.getByText(globString)).toBeInTheDocument();
    }
    // Shows group description changes.
    change = mockHistory.changes[2];
    expect(screen.getByText(change.description)).toBeInTheDocument();
  });
});
