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

import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { RecentPassesProvider } from '@/test_investigation/context';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';
import { mockFetchArtifactContent } from '@/testing_tools/mocks/artifact_mock';

import { ArtifactContentView } from './artifact_content_view';

describe('<ArtifactContentView />', () => {
  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  it('given an artifact, then should display the contents', async () => {
    const MOCK_ARTIFACT_URL =
      'http://mock.results.api.luci.app/artifact-content/test';
    const MOCK_ARTIFACT_CONTENT = 'test data';

    mockFetchArtifactContent(MOCK_ARTIFACT_URL, MOCK_ARTIFACT_CONTENT);

    render(
      <FakeContextProvider>
        <RecentPassesProvider passingResults={[]} error={null}>
          <ArtifactContentView
            artifact={Artifact.fromPartial({
              artifactId: 'test',
              name: 'test',
              contentType: 'text/plain',
              sizeBytes: '1024',
              fetchUrl: MOCK_ARTIFACT_URL,
            })}
          />
        </RecentPassesProvider>
      </FakeContextProvider>,
    );
    await waitFor(() =>
      expect(screen.getByText(MOCK_ARTIFACT_CONTENT)).toBeInTheDocument(),
    );
  });
});
