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

import { GrpcError, RpcCode } from '@chopsui/prpc-client';
import { act, render, screen } from '@testing-library/react';

import { CacheOption } from '@/generic_libs/tools/cached_fn';
import { BuilderItem } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';
import {
  BuildersClientImpl,
  GetBuilderRequest,
} from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_service.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { BuilderPage } from './builder_page';
import { EndedBuildsSection } from './ended_builds_section';
import { MachinePoolSection } from './machine_pool_section';

jest.mock('./machine_pool_section', () =>
  createSelectiveMockFromModule<typeof import('./machine_pool_section')>(
    './machine_pool_section',
    ['MachinePoolSection'],
  ),
);

jest.mock('./ended_builds_section', () =>
  createSelectiveMockFromModule<typeof import('./ended_builds_section')>(
    './ended_builds_section',
    ['EndedBuildsSection'],
  ),
);

jest.mock('./started_builds_section', () =>
  createSelectiveMockFromModule<typeof import('./started_builds_section')>(
    './started_builds_section',
    ['StartedBuildsSection'],
  ),
);

jest.mock('./pending_builds_section', () =>
  createSelectiveMockFromModule<typeof import('./pending_builds_section')>(
    './pending_builds_section',
    ['PendingBuildsSection'],
  ),
);

jest.mock('./views_section', () =>
  createSelectiveMockFromModule<typeof import('./views_section')>(
    './views_section',
    ['ViewsSection'],
  ),
);

describe('BuilderPage', () => {
  let getBuilderMock: jest.SpyInstance<
    Promise<BuilderItem>,
    [req: GetBuilderRequest, cacheOpt?: CacheOption | undefined]
  >;
  let mockedEndedBuildsSection: jest.MockedFunctionDeep<
    typeof EndedBuildsSection
  >;
  let mockedMachinePoolSection: jest.MockedFunctionDeep<
    typeof MachinePoolSection
  >;

  beforeEach(() => {
    jest.useFakeTimers();
    getBuilderMock = jest.spyOn(BuildersClientImpl.prototype, 'GetBuilder');
    mockedEndedBuildsSection = jest
      .mocked(EndedBuildsSection)
      .mockReturnValue(<>mocked ended builds section</>);
    mockedMachinePoolSection = jest.mocked(MachinePoolSection);
  });

  afterEach(() => {
    jest.useRealTimers();
    getBuilderMock.mockReset();
    mockedEndedBuildsSection.mockReset();
    mockedMachinePoolSection.mockReset();
  });

  test('should render correctly', async () => {
    getBuilderMock.mockResolvedValue(
      BuilderItem.fromPartial({
        id: { project: 'project', bucket: 'bucket', builder: 'builder' },
        config: {
          swarmingHost: 'https://swarming.host',
          descriptionHtml: 'some builder description',
          dimensions: ['10:key1:val1', '12:key2:val2'] as readonly string[],
        },
      }),
    );
    render(
      <FakeContextProvider
        mountedPath="/p/:project/builder/:bucket/:builder"
        routerOptions={{
          initialEntries: ['/p/project/builder/bucket/builder'],
        }}
      >
        <BuilderPage />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    expect(
      screen.queryByText(
        'Failed to query the builder. If you can see recent builds, the builder might have been deleted recently.',
      ),
    ).not.toBeInTheDocument();
    expect(screen.getByText('some builder description')).toBeInTheDocument();
    expect(mockedEndedBuildsSection).toHaveBeenCalledWith(
      {
        builderId: { project: 'project', bucket: 'bucket', builder: 'builder' },
      },
      expect.anything(),
    );
    expect(mockedMachinePoolSection).toHaveBeenCalledWith(
      {
        swarmingHost: 'https://swarming.host',
        dimensions: [
          { key: 'key1', value: 'val1' },
          { key: 'key2', value: 'val2' },
        ],
      },
      expect.anything(),
    );
  });

  test('failing to query builder should not break the builder page', async () => {
    getBuilderMock.mockRejectedValue(
      new GrpcError(RpcCode.NOT_FOUND, 'builder was removed'),
    );
    render(
      <FakeContextProvider
        mountedPath="/p/:project/builder/:bucket/:builder"
        routerOptions={{
          initialEntries: ['/p/project/builder/bucket/builder'],
        }}
      >
        <BuilderPage />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    expect(
      screen.getByText(
        'Failed to query the builder. If you can see recent builds, the builder might have been deleted recently.',
      ),
    ).toBeInTheDocument();
    // The ended builds section should still be displayed.
    expect(screen.getByText('mocked ended builds section')).toBeInTheDocument();
    expect(mockedEndedBuildsSection).toHaveBeenCalledWith(
      {
        builderId: { project: 'project', bucket: 'bucket', builder: 'builder' },
      },
      expect.anything(),
    );
    expect(mockedMachinePoolSection).not.toHaveBeenCalled();
  });
});
