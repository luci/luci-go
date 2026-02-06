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

import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import { QueuedStickyScrollingBase } from '@/generic_libs/components/queued_sticky';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { RecentPassesProvider } from '@/test_investigation/context';
import { InvocationProvider } from '@/test_investigation/context';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';
import {
  mockFetchHandler,
  mockFetchRaw,
  resetMockFetch,
} from '@/testing_tools/jest_utils';
import { mockFetchArtifactContent } from '@/testing_tools/mocks/artifact_mock';

import { ArtifactContentView } from './artifact_content_view';

const MOCK_RAW_INVOCATION_ID = 'inv-id-123';
const MOCK_PROJECT_ID = 'test-project';

// Mock scroll to to avoid errors in virtualized list.
window.scrollTo = jest.fn();

describe('<ArtifactContentView />', () => {
  afterEach(() => {
    resetMockFetch();
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
    // Should show filename
    await waitFor(() => expect(screen.getByText('test')).toBeInTheDocument());

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

    mockFetchHandler(MOCK_ARTIFACT_URL, async () => {
      await new Promise((resolve) => setTimeout(resolve, 100));
      return new Response(FULL_CONTENT, {
        headers: { 'Content-Type': 'text/plain' },
      });
    });

    mockFetchRaw(MOCK_ARTIFACT_URL + '?n=5000', INITIAL_CONTENT, {
      headers: { 'Content-Type': 'text/plain' },
    });

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

    // Should show initial content and loading notification first
    await waitFor(() => {
      expect(screen.getByText(INITIAL_CONTENT)).toBeInTheDocument();
      expect(
        screen.getByText(/Displaying preview. Still loading full/),
      ).toBeInTheDocument();
    });

    // Should eventually show full content and hide loading notification
    await waitFor(() => {
      expect(screen.getByText(FULL_CONTENT)).toBeInTheDocument();
      expect(
        screen.queryByText(/Displaying preview. Still loading full/),
      ).not.toBeInTheDocument();
    });
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

    mockFetchHandler(MOCK_GCS_URL, async () => {
      await new Promise((resolve) => setTimeout(resolve, 100));
      return new Response(FULL_CONTENT, {
        headers: { 'Content-Type': 'text/plain' },
      });
    });

    mockFetchHandler(
      (url, init) => {
        const headers = init?.headers;
        let range: string | null | undefined = null;
        if (headers instanceof Headers) {
          range = headers.get('Range') || headers.get('range');
        } else if (headers && !Array.isArray(headers)) {
          // Assume Record<string, string>
          const h = headers as Record<string, string>;
          range = h['Range'] || h['range'];
        }

        return url === MOCK_GCS_URL && range === 'bytes=0-4999';
      },
      async () => {
        return new Response(INITIAL_CONTENT, {
          headers: { 'Content-Type': 'text/plain' },
        });
      },
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

    // Should show initial content and loading notification first
    await waitFor(() => {
      expect(screen.getByText(INITIAL_CONTENT)).toBeInTheDocument();
      expect(
        screen.getByText(/Displaying preview. Still loading full/),
      ).toBeInTheDocument();
    });

    // Should eventually show full content and hide loading notification
    await waitFor(() => {
      expect(screen.getByText(FULL_CONTENT)).toBeInTheDocument();
      expect(
        screen.queryByText(/Displaying preview. Still loading full/),
      ).not.toBeInTheDocument();
    });
  });

  it('given a text diff artifact, then should display Open Raw option in formatted view', async () => {
    const MOCK_ARTIFACT_URL =
      'http://mock.results.api.luci.app/artifact-content/text_diff2';
    const MOCK_CONTENT = `diff --git a/file b/file
index 1..2
--- a/file
+++ b/file
@@ -1 +1 @@
-original
+modified`;
    const mockInvocation = Invocation.fromPartial({
      name: 'invocations/inv-123',
    });

    // Use exact match to avoid issues with function matcher
    // mockFetchArtifactContent uses getOnce, using get here to be safe against refetches
    mockFetchRaw(MOCK_ARTIFACT_URL + '?n=5000', MOCK_CONTENT, {
      headers: { 'Content-Type': 'text/plain' },
    });
    render(
      <FakeContextProvider>
        <InvocationProvider
          project="test-project"
          invocation={mockInvocation}
          rawInvocationId={MOCK_RAW_INVOCATION_ID}
          isLegacyInvocation={true}
        >
          <RecentPassesProvider passingResults={[]} error={null}>
            <ArtifactContentView
              artifact={Artifact.fromPartial({
                artifactId: 'text_diff',
                name: 'text_diff',
                contentType: 'text/x-diff',
                fetchUrl: MOCK_ARTIFACT_URL,
                sizeBytes: '100',
              })}
            />
          </RecentPassesProvider>
        </InvocationProvider>
      </FakeContextProvider>,
    );

    // Should show content
    await waitFor(
      () => expect(screen.getByText('modified')).toBeInTheDocument(),
      { timeout: 3000 },
    );

    // Should have the menu button
    const menuButton = screen.getByRole('button', { name: 'Options' });
    fireEvent.click(menuButton);

    // Should show Open Raw option
    expect(screen.getByText('Open Raw')).toBeInTheDocument();
  });

  it('given a binary artifact, then should display details and open raw link', async () => {
    const MOCK_ARTIFACT_URL =
      'http://mock.results.api.luci.app/artifact-content/binary';
    const MOCK_CONTENT = 'some binary content';
    const mockInvocation = Invocation.fromPartial({
      name: 'invocations/inv-123',
    });

    mockFetchArtifactContent(MOCK_ARTIFACT_URL + '?n=5000', MOCK_CONTENT);

    render(
      <FakeContextProvider>
        <InvocationProvider
          project="test-project"
          invocation={mockInvocation}
          rawInvocationId={MOCK_RAW_INVOCATION_ID}
          isLegacyInvocation={true}
        >
          <RecentPassesProvider passingResults={[]} error={null}>
            <ArtifactContentView
              artifact={Artifact.fromPartial({
                artifactId: 'binary.bin',
                name: 'binary.bin',
                contentType: 'application/octet-stream',
                fetchUrl: MOCK_ARTIFACT_URL,
                sizeBytes: '2048', // 2 KB
              })}
            />
          </RecentPassesProvider>
        </InvocationProvider>
      </FakeContextProvider>,
    );

    // Should show filename in header (using getAllByText because it appears in header and table)
    await waitFor(() =>
      expect(screen.getAllByText('binary.bin')[0]).toBeInTheDocument(),
    );

    // Should show "Preview not available" message
    expect(
      screen.getByText(
        'Preview not available for content type: application/octet-stream.',
      ),
    ).toBeInTheDocument();

    // Should show details table
    expect(screen.getByText('Name')).toBeInTheDocument();
    expect(screen.getByText('Size')).toBeInTheDocument();
    expect(screen.getByText('2.00 KB')).toBeInTheDocument(); // 2048 bytes
    expect(screen.getByText('Content Type')).toBeInTheDocument();
    expect(screen.getByText('application/octet-stream')).toBeInTheDocument();

    // Should show Open Raw link
    const link = screen.getByRole('link', { name: 'Open Raw Artifact' });
    expect(link).toBeInTheDocument();
    expect(link).toHaveAttribute('href', MOCK_ARTIFACT_URL);
  });
});
