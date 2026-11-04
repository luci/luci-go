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

import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react';

import { FakeAuthStateProvider } from '@/testing_tools/fakes/fake_auth_state_provider';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { SEMAPHORE_POLL_INTERVAL_MS } from './hooks/use_semaphore_messages';
import { SemaphoreBanner } from './semaphore_banner';

const SEMAPHORE_HOST = 'semaphore.example.com';
const SEMAPHORE_API_KEY = 'test-semaphore-api-key';

function jsonResponse(body: unknown): Response {
  return { ok: true, status: 200, json: async () => body } as Response;
}

function renderBanner(
  email = 'tester@google.com',
  realm: string | null = 'android:presubmit',
) {
  return render(
    <FakeContextProvider>
      <FakeAuthStateProvider value={{ identity: `user:${email}`, email }}>
        <SemaphoreBanner realm={realm === null ? undefined : realm} />
      </FakeAuthStateProvider>
    </FakeContextProvider>,
  );
}

describe('SemaphoreBanner', () => {
  const originalFetch = global.fetch;
  const originalSettings = SETTINGS;
  let mockFetch: jest.MockedFunction<typeof fetch>;

  beforeEach(() => {
    mockFetch = jest.fn();
    global.fetch = mockFetch;
    (self as unknown as { SETTINGS: typeof SETTINGS }).SETTINGS = Object.freeze(
      {
        ...originalSettings,
        chronicle: {
          semaphoreHost: SEMAPHORE_HOST,
          semaphoreApiKey: SEMAPHORE_API_KEY,
        },
      },
    );
  });

  afterEach(() => {
    jest.useRealTimers();
    global.fetch = originalFetch;
    (self as unknown as { SETTINGS: typeof SETTINGS }).SETTINGS =
      originalSettings;
  });

  it('renders nothing when there are no messages or banner is disabled', async () => {
    const { unmount } = renderBanner('user@example.com');
    expect(screen.queryByRole('region')).not.toBeInTheDocument();
    unmount();

    mockFetch.mockResolvedValue(jsonResponse({ messages: [] }));
    renderBanner();
    await waitFor(() => expect(mockFetch).toHaveBeenCalledTimes(1));
    expect(screen.queryByRole('region')).not.toBeInTheDocument();
  });

  it('shows enabled global messages, most severe first', async () => {
    mockFetch.mockResolvedValue(
      jsonResponse({
        messages: [
          { name: 'messages/1', shortMessage: 'Maintenance', urgency: 'INFO' },
          { name: 'messages/2', longMessage: 'Flaky tests', urgency: 'WARN' },
          { name: 'messages/3', longMessage: 'Build outage', urgency: 'FAIL' },
        ],
      }),
    );

    renderBanner();

    await waitFor(() => expect(screen.getAllByRole('alert')).toHaveLength(3));
    const alerts = screen.getAllByRole('alert');
    expect(alerts.map((alert) => alert.textContent)).toEqual([
      'Build outage',
      'Flaky tests',
      'Maintenance',
    ]);
    expect(alerts[0]).toHaveClass('MuiAlert-standardError');
    expect(alerts[1]).toHaveClass('MuiAlert-standardWarning');
    expect(alerts[2]).toHaveClass('MuiAlert-standardInfo');
  });

  it('dismisses individual messages and preserves dismissals across poll intervals', async () => {
    jest.useFakeTimers();
    mockFetch.mockResolvedValue(
      jsonResponse({
        messages: [
          { name: 'messages/1', longMessage: 'First', urgency: 'WARN' },
          { name: 'messages/2', longMessage: 'Second', urgency: 'INFO' },
        ],
      }),
    );

    renderBanner();

    const buttons = await screen.findAllByRole('button', {
      name: 'Dismiss Semaphore notification',
    });
    fireEvent.click(buttons[0]);
    expect(screen.queryByText('First')).not.toBeInTheDocument();
    expect(screen.getByText('Second')).toBeInTheDocument();

    mockFetch.mockResolvedValue(
      jsonResponse({
        messages: [
          { name: 'messages/1', longMessage: 'First', urgency: 'WARN' },
          { name: 'messages/2', longMessage: 'Second', urgency: 'INFO' },
          { name: 'messages/3', longMessage: 'Third', urgency: 'FAIL' },
        ],
      }),
    );

    await act(async () => {
      jest.advanceTimersByTime(SEMAPHORE_POLL_INTERVAL_MS);
    });

    expect(await screen.findByText('Third')).toBeInTheDocument();
    expect(screen.queryByText('First')).not.toBeInTheDocument();
    expect(screen.getByText('Second')).toBeInTheDocument();
  });

  it('linkifies shortlinks and URLs', async () => {
    mockFetch.mockResolvedValue(
      jsonResponse({
        messages: [
          {
            name: 'messages/1',
            longMessage:
              'Outage b/579526388 irm/i_abc123 omg/9876: retry after crrev.com/c/12345 lands, see go/semaphore.',
            urgency: 'FAIL',
          },
          {
            name: 'messages/2',
            longMessage:
              'Docs: https://ci.chromium.org/ui/p/chromium (not nab/123).',
            urgency: 'INFO',
          },
        ],
      }),
    );

    renderBanner();

    const links = await screen.findAllByRole('link');
    expect(
      links.map((link) => [link.textContent, link.getAttribute('href')]),
    ).toEqual([
      ['b/579526388', 'http://b/579526388'],
      ['irm/i_abc123', 'http://irm/i_abc123'],
      ['omg/9876', 'http://omg/9876'],
      ['crrev.com/c/12345', 'http://crrev.com/c/12345'],
      ['go/semaphore', 'http://go/semaphore'],
      [
        'https://ci.chromium.org/ui/p/chromium',
        'https://ci.chromium.org/ui/p/chromium',
      ],
    ]);
    expect(screen.getAllByRole('alert')[0]).toHaveTextContent(
      'Outage b/579526388 irm/i_abc123 omg/9876: retry after crrev.com/c/12345 lands, see go/semaphore.',
    );
    expect(screen.getAllByRole('alert')[1]).toHaveTextContent(
      'Docs: https://ci.chromium.org/ui/p/chromium (not nab/123).',
    );
  });
});
