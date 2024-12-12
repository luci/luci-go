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

import fetchMock from 'fetch-mock-jest';
import { createHandlerBoundToURL as workboxCreateHandlerBoundToURL } from 'workbox-precaching';

import { FakeExtendableEvent } from '@/testing_tools/fakes/fake_extendable_event';

import {
  createHandlerBoundToURL,
  _updateLastKnownFreshTime,
} from './stale_while_revalidate';

jest.mock('workbox-precaching', () =>
  self.createSelectiveMockFromModule<typeof import('workbox-precaching')>(
    'workbox-precaching',
    ['createHandlerBoundToURL'],
  ),
);

const cachedHtml = `
<!DOCTYPE html>

<head>My Cached HTML</head>
<body>
  <div>Body</div>
</body>`;

const freshHtml = `
<!DOCTYPE html>

<head>My Fresh HTML</head>
<body>
  <div>Body</div>
</body>
`;

const url = '/ui/index.html';

describe('createHandlerBoundToURL', () => {
  const wbMock = jest.mocked(workboxCreateHandlerBoundToURL);
  beforeEach(() => {
    jest.useFakeTimers();
    fetchMock.get(
      url,
      () =>
        new Response(freshHtml, { headers: { 'Content-Type': 'text/html' } }),
    );
    wbMock.mockReturnValue(
      async () =>
        new Response(cachedHtml, { headers: { 'Content-Type': 'text/html' } }),
    );
  });

  afterEach(() => {
    jest.useRealTimers();
    fetchMock.restore();
    wbMock.mockClear();
  });

  it('should return cached HTML when service worker is fresh', async () => {
    await _updateLastKnownFreshTime();
    const handler = createHandlerBoundToURL(url, {
      staleWhileRevalidate: 60000,
    });

    const request = new Request(url, { mode: 'navigate' });
    const response = await handler({
      event: new FakeExtendableEvent('fetch'),
      request,
      url: new URL(url, 'http://localhost:1234'),
    });
    expect(await response.text()).toEqual(cachedHtml);
  });

  it('should return cached HTML when service worker is non-fresh', async () => {
    await _updateLastKnownFreshTime();
    const handler = createHandlerBoundToURL(url, {
      staleWhileRevalidate: 60000,
    });

    await jest.advanceTimersByTimeAsync(60000 * 2);

    const request = new Request(url, { mode: 'navigate' });
    const response = await handler({
      event: new FakeExtendableEvent('fetch'),
      request,
      url: new URL(url, 'http://localhost:1234'),
    });
    expect(await response.text()).toEqual(freshHtml);
  });

  it('should return cached HTML when service worker was considered fresh since warm-up', async () => {
    await _updateLastKnownFreshTime();
    const handler = createHandlerBoundToURL(url, {
      staleWhileRevalidate: 60000,
    });

    await jest.advanceTimersByTimeAsync(60000 * 2);

    // Was not fresh.
    const request = new Request(url, { mode: 'navigate' });
    let response = await handler({
      event: new FakeExtendableEvent('fetch'),
      request,
      url: new URL(url, 'http://localhost:1234'),
    });
    expect(await response.text()).toEqual(freshHtml);

    // Confirmed fresh again.
    await _updateLastKnownFreshTime();
    response = await handler({
      event: new FakeExtendableEvent('fetch'),
      request,
      url: new URL(url, 'http://localhost:1234'),
    });
    expect(await response.text()).toEqual(cachedHtml);

    // No longer fresh again.
    await jest.advanceTimersByTimeAsync(60000 * 2);
    await _updateLastKnownFreshTime();
    response = await handler({
      event: new FakeExtendableEvent('fetch'),
      request,
      url: new URL(url, 'http://localhost:1234'),
    });
    // But still return cached since it was once fresh after warm-up.
    expect(await response.text()).toEqual(cachedHtml);
  });
});
