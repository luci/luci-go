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

import { fetchSemaphoreMessages } from './semaphore';

const SEMAPHORE_HOST = 'semaphore.example.com';
const SEMAPHORE_API_KEY = 'test-semaphore-api-key';

function jsonResponse(body: unknown): Response {
  return { ok: true, status: 200, json: async () => body } as Response;
}

describe('fetchSemaphoreMessages', () => {
  const originalFetch = global.fetch;
  let mockFetch: jest.MockedFunction<typeof fetch>;

  beforeEach(() => {
    mockFetch = jest.fn();
    global.fetch = mockFetch;
  });

  afterEach(() => {
    global.fetch = originalFetch;
  });

  it('fetches enabled global messages and sorts most severe first', async () => {
    mockFetch.mockResolvedValue(
      jsonResponse({
        messages: [
          { name: 'messages/1', shortMessage: 'Maintenance', urgency: 'INFO' },
          { name: 'messages/2', longMessage: 'Default warn' },
          { name: 'messages/3', longMessage: 'Build outage', urgency: 'FAIL' },
          { name: 'messages/4', longMessage: 'Flaky tests', urgency: 'WARN' },
        ],
      }),
    );

    const messages = await fetchSemaphoreMessages(
      SEMAPHORE_HOST,
      SEMAPHORE_API_KEY,
    );

    expect(mockFetch).toHaveBeenCalledTimes(1);
    expect(mockFetch).toHaveBeenCalledWith(
      `https://${SEMAPHORE_HOST}/v4/messages?enabled=true&scopes=global&key=${SEMAPHORE_API_KEY}`,
      { credentials: 'include' },
    );
    expect(messages.map((m) => m.name)).toEqual([
      'messages/3',
      'messages/2',
      'messages/4',
      'messages/1',
    ]);
  });

  it('returns an empty array when the response omits messages', async () => {
    mockFetch.mockResolvedValue(jsonResponse({}));

    await expect(
      fetchSemaphoreMessages(SEMAPHORE_HOST, SEMAPHORE_API_KEY),
    ).resolves.toEqual([]);
  });

  it('retries once after refreshing ÜberProxy cookies on a network TypeError', async () => {
    mockFetch
      .mockRejectedValueOnce(new TypeError('Failed to fetch'))
      .mockResolvedValueOnce({ type: 'opaque' } as Response)
      .mockResolvedValueOnce(
        jsonResponse({
          messages: [
            { name: 'messages/1', longMessage: 'Recovered', urgency: 'WARN' },
          ],
        }),
      );

    const messages = await fetchSemaphoreMessages(
      SEMAPHORE_HOST,
      SEMAPHORE_API_KEY,
    );

    expect(messages).toEqual([
      { name: 'messages/1', longMessage: 'Recovered', urgency: 'WARN' },
    ]);
    expect(mockFetch).toHaveBeenCalledTimes(3);
    expect(mockFetch).toHaveBeenNthCalledWith(
      2,
      `https://${SEMAPHORE_HOST}/js/empty.js`,
      {
        method: 'GET',
        credentials: 'include',
        mode: 'no-cors',
      },
    );
  });

  it('does not refresh cookies on non-OK HTTP responses and propagates retry failures', async () => {
    mockFetch.mockResolvedValueOnce({ ok: false, status: 403 } as Response);

    await expect(
      fetchSemaphoreMessages(SEMAPHORE_HOST, SEMAPHORE_API_KEY),
    ).rejects.toThrow('Semaphore API returned HTTP 403');
    expect(mockFetch).toHaveBeenCalledTimes(1);

    mockFetch.mockReset();
    mockFetch
      .mockRejectedValueOnce(new TypeError('Failed to fetch'))
      .mockResolvedValueOnce({ type: 'opaque' } as Response)
      .mockRejectedValueOnce(new TypeError('Failed to fetch'));

    await expect(
      fetchSemaphoreMessages(SEMAPHORE_HOST, SEMAPHORE_API_KEY),
    ).rejects.toThrow('Failed to fetch');
    expect(mockFetch).toHaveBeenCalledTimes(3);
  });
});
