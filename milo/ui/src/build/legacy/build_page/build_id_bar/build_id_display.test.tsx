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

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { BuildIdDisplay } from './build_id_display';

describe('<BuildIdDisplay />', () => {
  afterEach(() => {
    cleanup();
  });

  it('should render help icon when builder description is specified', async () => {
    render(
      <FakeContextProvider>
        <BuildIdDisplay
          builderId={{
            project: 'project',
            bucket: 'fake-bucket',
            builder: 'fake-builder',
          }}
          buildNumOrId="b654321"
          builderDescription="this is a fake builder"
        />
      </FakeContextProvider>,
    );

    expect(screen.getByTestId('HelpIcon')).toBeInTheDocument();
  });

  it('should not render help icon when builder description is not specified', async () => {
    render(
      <FakeContextProvider>
        <BuildIdDisplay
          builderId={{
            project: 'project',
            bucket: 'fake-bucket',
            builder: 'fake-builder',
          }}
          buildNumOrId="b654321"
        />
      </FakeContextProvider>,
    );

    expect(screen.queryByTestId('HelpIcon')).not.toBeInTheDocument();
  });
});
