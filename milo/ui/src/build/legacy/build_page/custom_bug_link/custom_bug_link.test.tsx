// Copyright 2023 The LUCI Authors.
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
import { act } from 'react';

import { Project } from '@/proto/go.chromium.org/luci/milo/proto/projectconfig/project.pb';
import {
  GetProjectCfgRequest,
  MiloInternalClientImpl,
} from '@/proto/go.chromium.org/luci/milo/proto/v1/rpc.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { CustomBugLink } from './custom_bug_link';

describe('<CustomBugLink />', () => {
  let getProjectCfgMock: jest.SpiedFunction<
    MiloInternalClientImpl['GetProjectCfg']
  >;

  beforeEach(() => {
    jest.useFakeTimers();
    getProjectCfgMock = jest
      .spyOn(MiloInternalClientImpl.prototype, 'GetProjectCfg')
      .mockResolvedValue(
        Project.fromPartial({
          bugUrlTemplate:
            'https://b.corp.google.com/createIssue?description=builder%3A{{{milo_builder_url}}}build%3A{{{milo_build_url}}}',
        }),
      );
  });

  afterEach(() => {
    jest.useRealTimers();
    getProjectCfgMock.mockClear();
  });

  test('e2e', async () => {
    const { rerender } = render(
      <FakeContextProvider>
        <CustomBugLink project="proj" />
      </FakeContextProvider>,
    );
    await act(() => jest.runAllTimersAsync());

    // The query is sent even when `build` is not yet populated.
    expect(getProjectCfgMock).toHaveBeenCalledWith(
      GetProjectCfgRequest.fromPartial({ project: 'proj' }),
    );
    expect(screen.queryByText('File a bug')).not.toBeInTheDocument();

    rerender(
      <FakeContextProvider>
        <CustomBugLink
          project="proj"
          build={{
            id: '1234',
            builder: { project: 'proj', bucket: 'bucket', builder: 'builder' },
          }}
        />
      </FakeContextProvider>,
    );

    // The bug link is only rendered when `build` is populated.
    expect(screen.getByText('File a bug')).toBeInTheDocument();
  });
});
