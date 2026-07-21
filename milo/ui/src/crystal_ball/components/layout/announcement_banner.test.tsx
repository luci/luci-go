// Copyright 2026 The LUCI Authors.
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

import { fireEvent, render, screen } from '@testing-library/react';

import {
  Announcement,
  Announcement_Severity,
  Announcement_State,
  Announcement_Visibility,
  ListAnnouncementsResponse,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { AnnouncementBanner } from './announcement_banner';

const mockedUseListAnnouncements = jest.fn();
const mockedUseAuthState = jest.fn();

jest.mock('@/crystal_ball/hooks/use_announcements_api', () => ({
  useListAnnouncements: (...args: unknown[]) =>
    mockedUseListAnnouncements(...args),
}));

jest.mock('@/common/components/auth_state_provider', () => ({
  ...jest.requireActual('@/common/components/auth_state_provider'),
  useAuthState: () => mockedUseAuthState(),
}));

describe('AnnouncementBanner', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockedUseAuthState.mockReturnValue({
      email: 'user@google.com',
    });
  });

  it('renders active internal or both announcements for internal users', () => {
    mockedUseListAnnouncements.mockReturnValue({
      data: ListAnnouncementsResponse.fromPartial({
        announcements: [
          Announcement.fromPartial({
            name: 'announcements/1',
            message: 'Internal Maintenance Scheduled',
            state: Announcement_State.ACTIVE,
            visibility: Announcement_Visibility.INTERNAL,
            severity: Announcement_Severity.WARNING,
          }),
          Announcement.fromPartial({
            name: 'announcements/2',
            message: 'External Announcement Should Not Render',
            state: Announcement_State.ACTIVE,
            visibility: Announcement_Visibility.EXTERNAL,
            severity: Announcement_Severity.INFO,
          }),
        ],
      }),
    });

    render(
      <FakeContextProvider>
        <AnnouncementBanner />
      </FakeContextProvider>,
    );

    expect(
      screen.getByText('Internal Maintenance Scheduled'),
    ).toBeInTheDocument();
    expect(
      screen.queryByText('External Announcement Should Not Render'),
    ).not.toBeInTheDocument();
  });

  it('allows dismissing an announcement', () => {
    mockedUseListAnnouncements.mockReturnValue({
      data: ListAnnouncementsResponse.fromPartial({
        announcements: [
          Announcement.fromPartial({
            name: 'announcements/1',
            message: 'Dismissible Notice',
            state: Announcement_State.ACTIVE,
            visibility: Announcement_Visibility.BOTH,
            severity: Announcement_Severity.INFO,
          }),
        ],
      }),
    });

    render(
      <FakeContextProvider>
        <AnnouncementBanner />
      </FakeContextProvider>,
    );

    expect(screen.getByText('Dismissible Notice')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /dismiss/i }));
    expect(screen.queryByText('Dismissible Notice')).not.toBeInTheDocument();
  });

  it('renders active external announcements for external users', () => {
    mockedUseAuthState.mockReturnValue({
      email: 'user@example.com',
    });

    mockedUseListAnnouncements.mockReturnValue({
      data: ListAnnouncementsResponse.fromPartial({
        announcements: [
          Announcement.fromPartial({
            name: 'announcements/1',
            message: 'External Announcement Should Render',
            state: Announcement_State.ACTIVE,
            visibility: Announcement_Visibility.EXTERNAL,
            severity: Announcement_Severity.INFO,
          }),
          Announcement.fromPartial({
            name: 'announcements/2',
            message: 'Internal Announcement Should Not Render',
            state: Announcement_State.ACTIVE,
            visibility: Announcement_Visibility.INTERNAL,
            severity: Announcement_Severity.WARNING,
          }),
        ],
      }),
    });

    render(
      <FakeContextProvider>
        <AnnouncementBanner />
      </FakeContextProvider>,
    );

    expect(
      screen.getByText('External Announcement Should Render'),
    ).toBeInTheDocument();
    expect(
      screen.queryByText('Internal Announcement Should Not Render'),
    ).not.toBeInTheDocument();
  });
});
