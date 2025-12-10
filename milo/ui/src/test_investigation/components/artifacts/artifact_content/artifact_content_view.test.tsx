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

import { render, screen, waitFor } from '@testing-library/react';
import fetchMock from 'fetch-mock-jest';

import { QueuedStickyScrollingBase } from '@/generic_libs/components/queued_sticky';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { RecentPassesProvider } from '@/test_investigation/context';
import { InvocationProvider } from '@/test_investigation/context';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';
import { mockFetchArtifactContent } from '@/testing_tools/mocks/artifact_mock';

import { ArtifactContentView } from './artifact_content_view';

const MOCK_RAW_INVOCATION_ID = 'inv-id-123';
const MOCK_PROJECT_ID = 'test-project';

describe('<ArtifactContentView />', () => {
  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  it('given an artifact, then should display the contents', async () => {
    const MOCK_ARTIFACT_URL =
      'http://mock.results.api.luci.app/artifact-content/test';
    const MOCK_ARTIFACT_CONTENT = 'test data';
    const mockInvocation = Invocation.fromPartial({
      name: 'invocations/inv-123',
      realm: `${MOCK_PROJECT_ID}:some-realm`,
      sourceSpec: { sources: { gitilesCommit: { position: '105' } } },
    });

    mockFetchArtifactContent(
      MOCK_ARTIFACT_URL + '?n=5000',
      MOCK_ARTIFACT_CONTENT,
    );

    render(
      <FakeContextProvider>
        <InvocationProvider
          project="test-project"
          invocation={mockInvocation}
          rawInvocationId={MOCK_RAW_INVOCATION_ID}
          isLegacyInvocation={true}
        >
          <RecentPassesProvider passingResults={[]} error={null}>
            <QueuedStickyScrollingBase>
              <ArtifactContentView
                artifact={Artifact.fromPartial({
                  artifactId: 'test',
                  name: 'test',
                  contentType: 'text/plain',
                  sizeBytes: '1024',
                  fetchUrl: MOCK_ARTIFACT_URL,
                })}
              />
            </QueuedStickyScrollingBase>
          </RecentPassesProvider>
        </InvocationProvider>
      </FakeContextProvider>,
    );
    await waitFor(() =>
      expect(screen.getByText(MOCK_ARTIFACT_CONTENT)).toBeInTheDocument(),
    );
  });

  it('given a large artifact, then should fetch initial and full content', async () => {
    const MOCK_ARTIFACT_URL =
      'http://mock.results.api.luci.app/artifact-content/large';
    const INITIAL_CONTENT = 'initial data';
    const FULL_CONTENT = 'initial data and more';
    const mockInvocation = Invocation.fromPartial({
      name: 'invocations/inv-123',
      realm: `${MOCK_PROJECT_ID}:some-realm`,
      sourceSpec: { sources: { gitilesCommit: { position: '105' } } },
    });

    fetchMock.getOnce(MOCK_ARTIFACT_URL + '?n=5000', {
      body: INITIAL_CONTENT,
      headers: { 'Content-Type': 'text/plain' },
    });
    fetchMock.getOnce(
      MOCK_ARTIFACT_URL,
      {
        body: FULL_CONTENT,
        headers: { 'Content-Type': 'text/plain' },
      },
      { delay: 100 },
    );

    render(
      <FakeContextProvider>
        <InvocationProvider
          project="test-project"
          invocation={mockInvocation}
          rawInvocationId={MOCK_RAW_INVOCATION_ID}
          isLegacyInvocation={true}
        >
          <RecentPassesProvider passingResults={[]} error={null}>
            <QueuedStickyScrollingBase>
              <ArtifactContentView
                artifact={Artifact.fromPartial({
                  artifactId: 'large',
                  name: 'large',
                  contentType: 'text/plain',
                  sizeBytes: '10000',
                  fetchUrl: MOCK_ARTIFACT_URL,
                })}
              />
            </QueuedStickyScrollingBase>
          </RecentPassesProvider>
        </InvocationProvider>
      </FakeContextProvider>,
    );

    // Should show initial content first
    await waitFor(() =>
      expect(screen.getByText(INITIAL_CONTENT)).toBeInTheDocument(),
    );

    // Should show loading notification
    expect(
      screen.getByText(/Displaying preview. Still loading full/),
    ).toBeInTheDocument();

    // Should eventually show full content
    await waitFor(() =>
      expect(screen.getByText(FULL_CONTENT)).toBeInTheDocument(),
    );

    // Loading notification should disappear
    expect(
      screen.queryByText(/Displaying preview. Still loading full/),
    ).not.toBeInTheDocument();
  });

  it('given a large artifact with GCS signed URL, then should use Range header', async () => {
    const MOCK_GCS_URL =
      'https://storage.googleapis.com/test-bucket/large?X-Goog-Signature=123';
    const INITIAL_CONTENT = 'initial data';
    const FULL_CONTENT = 'initial data and more';
    const mockInvocation = Invocation.fromPartial({
      name: 'invocations/inv-123',
      realm: `${MOCK_PROJECT_ID}:some-realm`,
      sourceSpec: { sources: { gitilesCommit: { position: '105' } } },
    });

    fetchMock.getOnce(
      (url, options) => {
        const opts = options as RequestInit;
        return !!(
          url === MOCK_GCS_URL &&
          opts.headers &&
          (opts.headers as Record<string, string>)['Range'] === 'bytes=0-4999'
        );
      },
      {
        body: INITIAL_CONTENT,
        headers: { 'Content-Type': 'text/plain' },
      },
    );
    fetchMock.getOnce(
      MOCK_GCS_URL,
      {
        body: FULL_CONTENT,
        headers: { 'Content-Type': 'text/plain' },
      },
      { delay: 100 },
    );

    render(
      <FakeContextProvider>
        <InvocationProvider
          project="test-project"
          invocation={mockInvocation}
          rawInvocationId={MOCK_RAW_INVOCATION_ID}
          isLegacyInvocation={true}
        >
          <RecentPassesProvider passingResults={[]} error={null}>
            <QueuedStickyScrollingBase>
              <ArtifactContentView
                artifact={Artifact.fromPartial({
                  artifactId: 'large-gcs',
                  name: 'large-gcs',
                  contentType: 'text/plain',
                  sizeBytes: '10000',
                  fetchUrl: MOCK_GCS_URL,
                })}
              />
            </QueuedStickyScrollingBase>
          </RecentPassesProvider>
        </InvocationProvider>
      </FakeContextProvider>,
    );

    // Should show initial content first
    await waitFor(() =>
      expect(screen.getByText(INITIAL_CONTENT)).toBeInTheDocument(),
    );

    // Should eventually show full content
    await waitFor(() =>
      expect(screen.getByText(FULL_CONTENT)).toBeInTheDocument(),
    );
  });
});
