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

import { cleanup, render, screen } from '@testing-library/react';
import fetchMock from 'fetch-mock-jest';
import { act } from 'react';

import { ARTIFACT_LENGTH_LIMIT } from '@/common/constants/verdict';
import { getRawArtifactURLPath } from '@/common/tools/url_utils';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ArtifactContextProvider } from '../context';

import { mockFetchTextArtifact } from './testing_tools/text_artifact_mock';
import { TextArtifact } from './text_artifact';

describe('<TextArtifact />', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    cleanup();
    fetchMock.mockClear();
    fetchMock.reset();
    jest.useRealTimers();
  });

  it('shows artifact', async () => {
    mockFetchTextArtifact(
      'invocations/inv-1/tests/test-1/results/result-1/artifacts/artifact-1',
      'short content',
    );
    render(
      <FakeContextProvider>
        <ArtifactContextProvider resultName="invocations/inv-1/tests/test-1/results/result-1">
          <TextArtifact artifactId="artifact-1" />
        </ArtifactContextProvider>
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    const url = new URL(fetchMock.lastCall()![0]);
    expect(parseInt(url.searchParams.get('n')!)).toBeGreaterThanOrEqual(
      ARTIFACT_LENGTH_LIMIT,
    );

    expect(screen.getByTestId('text-artifact-content')).toHaveTextContent(
      'short content',
    );
  });

  it('shows ANSI artifact', async () => {
    mockFetchTextArtifact(
      'invocations/inv-1/tests/test-1/results/result-1/artifacts/artifact-1',
      'magic-string:\u001b[36m<div data-testid="should-be-escaped">\u001b[39m\n\u001b[36m</div>\u001b[39m',
      'text/x-ansi',
    );
    render(
      <FakeContextProvider>
        <ArtifactContextProvider resultName="invocations/inv-1/tests/test-1/results/result-1">
          <TextArtifact artifactId="artifact-1" experimentalANSISupport />
        </ArtifactContextProvider>
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    const url = new URL(fetchMock.lastCall()![0]);
    expect(parseInt(url.searchParams.get('n')!)).toBeGreaterThanOrEqual(
      ARTIFACT_LENGTH_LIMIT,
    );

    // The ANSI sequence following the magic string should be converted to an
    // HTML tag (beginning with a `<`).
    expect(screen.getByTestId('text-artifact-content').innerHTML).toContain(
      'magic-string:<span',
    );
    // The HTML in the content should've been escaped.
    expect(screen.queryByTestId('should-be-escaped')).not.toBeInTheDocument();
  });

  it('shows invocation-level artifact', async () => {
    mockFetchTextArtifact(
      'invocations/inv-1/artifacts/artifact-1',
      'invocation level artifact content',
    );
    render(
      <FakeContextProvider>
        <ArtifactContextProvider resultName="invocations/inv-1/tests/test-1/results/result-1">
          <TextArtifact artifactId="artifact-1" invLevel />
        </ArtifactContextProvider>
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    const url = new URL(fetchMock.lastCall()![0]);
    expect(parseInt(url.searchParams.get('n')!)).toBeGreaterThanOrEqual(
      ARTIFACT_LENGTH_LIMIT,
    );

    expect(screen.getByTestId('text-artifact-content')).toHaveTextContent(
      'invocation level artifact content',
    );
  });

  it('shows a full link when artifact is truncated', async () => {
    mockFetchTextArtifact(
      'invocations/inv-1/tests/test-1/results/result-1/artifacts/artifact-1',
      'looooooooong content',
      'text/plain',
      ARTIFACT_LENGTH_LIMIT + 4,
    );
    render(
      <FakeContextProvider>
        <ArtifactContextProvider resultName="invocations/inv-1/tests/test-1/results/result-1">
          <TextArtifact artifactId="artifact-1" />
        </ArtifactContextProvider>
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    const url = new URL(fetchMock.lastCall()![0]);
    expect(parseInt(url.searchParams.get('n')!)).toBeGreaterThanOrEqual(
      ARTIFACT_LENGTH_LIMIT,
    );

    expect(screen.getByTestId('text-artifact-content').textContent).toContain(
      'looooooooong conten...',
    );
    expect(screen.getByRole('link')).toHaveAttribute(
      'href',
      getRawArtifactURLPath(
        'invocations/inv-1/tests/test-1/results/result-1/artifacts/artifact-1',
      ),
    );
  });

  it('shows artifact is empty when its empty', async () => {
    mockFetchTextArtifact(
      'invocations/inv-1/tests/test-1/results/result-1/artifacts/artifact-1',
      '',
    );
    render(
      <FakeContextProvider>
        <ArtifactContextProvider resultName="invocations/inv-1/tests/test-1/results/result-1">
          <TextArtifact artifactId="artifact-1" />
        </ArtifactContextProvider>
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    const url = new URL(fetchMock.lastCall()![0]);
    expect(parseInt(url.searchParams.get('n')!)).toBeGreaterThanOrEqual(
      ARTIFACT_LENGTH_LIMIT,
    );

    expect(screen.getByText((s) => s.includes('is empty'))).toBeInTheDocument();
  });

  it('shows alert when artifact failed to load', async () => {
    fetchMock.get(
      (urlStr) => {
        const url = new URL(urlStr);
        return (
          url.host === location.host &&
          url.pathname ===
            getRawArtifactURLPath(
              'invocations/inv-1/tests/test-1/results/result-1/artifacts/artifact-1',
            )
        );
      },
      {
        body: 'failed to load content',
        status: 404,
      },
      { overwriteRoutes: true },
    );
    render(
      <FakeContextProvider>
        <ArtifactContextProvider resultName="invocations/inv-1/tests/test-1/results/result-1">
          <TextArtifact artifactId="artifact-1" />
        </ArtifactContextProvider>
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    const url = new URL(fetchMock.lastCall()![0]);
    expect(parseInt(url.searchParams.get('n')!)).toBeGreaterThanOrEqual(
      ARTIFACT_LENGTH_LIMIT,
    );

    expect(
      screen.getByText((s) =>
        s.includes('Failed to load artifact: artifact-1'),
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText((s) => s.includes('failed to load content')),
    ).toBeInTheDocument();
  });
});
