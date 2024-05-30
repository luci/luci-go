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

import { render, screen } from '@testing-library/react';
import fetchMockJest from 'fetch-mock-jest';
import { act } from 'react';

import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { mockFetchTextArtifact } from '../artifact_tags/text_artifact/testing_tools/text_artifact_mock';

import { ArtifactLink } from './artifact_link';

describe('<ArtifactLink />', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
    fetchMockJest.reset();
  });

  it('should render a link to the artifact', () => {
    render(
      <FakeContextProvider>
        <ArtifactLink
          artifact={Artifact.fromPartial({
            artifactId: 'artifact1',
            name: 'invocations/inv1/artifacts/artifact1',
            contentType: 'text/plain',
          })}
          label="the artifact"
        />
      </FakeContextProvider>,
    );
    const link = screen.getByRole('link');
    expect(link).toHaveTextContent('the artifact');
    expect(link).toHaveAttribute(
      'href',
      '/raw-artifact/invocations/inv1/artifacts/artifact1',
    );
  });

  it('should support link artifact', async () => {
    mockFetchTextArtifact(
      'invocations/inv1/artifacts/artifact1',
      'https://tests.chromeos.goog/a/link/to/somewhere',
    );
    render(
      <FakeContextProvider>
        <ArtifactLink
          artifact={Artifact.fromPartial({
            artifactId: 'artifact1',
            name: 'invocations/inv1/artifacts/artifact1',
            contentType: 'text/x-uri',
          })}
          label="the artifact"
        />
      </FakeContextProvider>,
    );
    await act(() => jest.runAllTimersAsync());

    const link = screen.getByRole('link');
    expect(link).toHaveTextContent('the artifact');
    expect(link).toHaveAttribute(
      'href',
      'https://tests.chromeos.goog/a/link/to/somewhere',
    );
  });
});
