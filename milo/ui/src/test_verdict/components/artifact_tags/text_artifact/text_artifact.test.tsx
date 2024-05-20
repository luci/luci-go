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

import { act, cleanup, render, screen } from '@testing-library/react';
import fetchMock from 'fetch-mock-jest';

import { ARTIFACT_LENGTH_LIMIT } from '@/common/constants/test';
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
});
