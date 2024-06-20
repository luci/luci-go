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

import { OutputBuild } from '@/build/types';
import { Build } from '@/proto/go.chromium.org/luci/buildbucket/proto/build.pb';
import { Status } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { BuildContextProvider } from '../context';

import { BuildIdBar } from './build_id_bar';

const mockBuild = Build.fromPartial({
  builder: {
    project: 'project',
    bucket: 'real-bucket',
    builder: 'real-builder',
  },
  number: 1234,
  id: '654321',
  createTime: '2020-12-20',
  status: Status.SCHEDULED,
}) as OutputBuild;

describe('<BuildIdBar />', () => {
  afterEach(() => {
    cleanup();
  });

  // This may happen when user lands on the page with a build ID but incorrect
  // builder ID supplied by the URL.
  it('should render the real builder ID', () => {
    const { rerender } = render(
      <FakeContextProvider>
        <BuildContextProvider>
          <BuildIdBar
            builderId={{
              project: 'project',
              bucket: 'fake-bucket',
              builder: 'fake-builder',
            }}
            buildNumOrId="b654321"
          />
        </BuildContextProvider>
      </FakeContextProvider>,
    );

    expect(screen.getByText('fake-bucket')).toBeInTheDocument();
    expect(screen.getByText('fake-builder')).toBeInTheDocument();
    expect(screen.getByText('b654321')).toBeInTheDocument();

    rerender(
      <FakeContextProvider>
        <BuildContextProvider build={mockBuild}>
          <BuildIdBar
            builderId={{
              project: 'project',
              bucket: 'fake-bucket',
              builder: 'fake-builder',
            }}
            buildNumOrId="b654321"
          />
        </BuildContextProvider>
      </FakeContextProvider>,
    );

    expect(screen.queryByText('fake-bucket')).not.toBeInTheDocument();
    expect(screen.queryByText('fake-builder')).not.toBeInTheDocument();
    expect(screen.queryByText('b654321')).not.toBeInTheDocument();

    expect(screen.getByText('real-bucket')).toBeInTheDocument();
    expect(screen.getByText('real-builder')).toBeInTheDocument();
    expect(screen.getByText('1234')).toBeInTheDocument();
  });
});
