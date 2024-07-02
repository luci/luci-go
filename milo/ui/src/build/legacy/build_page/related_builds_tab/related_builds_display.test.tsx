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

import { cleanup, render } from '@testing-library/react';
import { act } from 'react';

import { PARTIAL_BUILD_FIELD_MASK } from '@/build/constants';
import { OutputBuild } from '@/build/types';
import { NEVER_PROMISE } from '@/common/constants/utils';
import { CategoryTree } from '@/generic_libs/tools/category_tree';
import { Build } from '@/proto/go.chromium.org/luci/buildbucket/proto/build.pb';
import {
  BuildsClientImpl,
  SearchBuildsRequest,
  SearchBuildsResponse,
} from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';
import { Status } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { RelatedBuildTable } from './related_build_table';
import { RelatedBuildsDisplay } from './related_builds_display';

function createMockBuild(
  id: string,
  ancestorIds: readonly string[] = [],
): Build {
  return Build.fromPartial({
    id,
    builder: {
      bucket: 'buck',
      builder: 'builder',
      project: 'proj',
    },
    ancestorIds,
    status: Status.SUCCESS,
    createTime: '2020-01-01',
  });
}

jest.mock('./related_build_table', () => {
  return createSelectiveSpiesFromModule<typeof import('./related_build_table')>(
    './related_build_table',
    ['RelatedBuildTable'],
  );
});

describe('RelatedBuildsDisplay', () => {
  let searchBuildsMock: jest.SpiedFunction<BuildsClientImpl['SearchBuilds']>;
  let relatedBuildsTableSpy: jest.MockedFunction<typeof RelatedBuildTable>;
  beforeEach(() => {
    jest.useFakeTimers();
    relatedBuildsTableSpy = jest.mocked(RelatedBuildTable);
    searchBuildsMock = jest.spyOn(BuildsClientImpl.prototype, 'SearchBuilds');
  });

  afterEach(() => {
    cleanup();
    jest.useRealTimers();
    searchBuildsMock.mockReset();
    relatedBuildsTableSpy.mockReset();
  });

  it('no buildset no descendant', async () => {
    searchBuildsMock.mockResolvedValue(SearchBuildsResponse.fromPartial({}));
    const selfBuild = Build.fromPartial({
      id: '12345',
      builder: {
        project: 'proj',
        bucket: 'bucket',
        builder: 'builder',
      },
      ancestorIds: [],
      tags: [{ key: 'otherkey', value: 'othervalue' }],
    }) as OutputBuild;
    render(
      <FakeContextProvider>
        <RelatedBuildsDisplay build={selfBuild} />
      </FakeContextProvider>,
    );

    await act(() => jest.advanceTimersByTimeAsync(1000));

    expect(searchBuildsMock).toHaveBeenCalledTimes(1);
    expect(searchBuildsMock).toHaveBeenNthCalledWith(
      1,
      SearchBuildsRequest.fromPartial({
        predicate: {
          descendantOf: '12345',
        },
        pageSize: searchBuildsMock.mock.calls[0][0].pageSize,
        mask: { fields: PARTIAL_BUILD_FIELD_MASK },
      }),
    );

    expect(relatedBuildsTableSpy).toHaveBeenLastCalledWith(
      {
        selfBuild: selfBuild,
        buildTree: new CategoryTree([
          [[...selfBuild.ancestorIds, selfBuild.id], selfBuild],
        ]),
        showLoadingBar: false,
      },
      expect.anything(),
    );
  });

  it('can dedupe builds', async () => {
    const selfBuild = Build.fromPartial({
      id: '00004',
      builder: {
        project: 'proj',
        bucket: 'bucket',
        builder: 'builder',
      },
      ancestorIds: ['00002'],
      tags: [
        { key: 'otherkey', value: 'othervalue' },
        { key: 'buildset', value: 'commit/git/1234' },
        { key: 'buildset', value: 'commit/gitiles/1234' },
        { key: 'buildset', value: 'commit/git/5678' },
        { key: 'buildset', value: 'commit/gitiles/5678' },
      ],
    }) as OutputBuild;

    searchBuildsMock.mockImplementation(async (req) => {
      if (
        req.predicate?.build?.startBuildId === '00002' &&
        req.predicate?.build?.endBuildId === '00002'
      ) {
        return SearchBuildsResponse.fromPartial({
          builds: [createMockBuild('00002')],
        });
      }

      if (req.predicate?.descendantOf === '00002') {
        return SearchBuildsResponse.fromPartial({
          builds: [
            createMockBuild('00003'),
            createMockBuild('00004', ['00002']),
            createMockBuild('00005'),
          ],
        });
      }

      if (req.predicate?.tags[0].value === 'commit/gitiles/1234') {
        return SearchBuildsResponse.fromPartial({
          builds: [createMockBuild('00001'), createMockBuild('00002')],
        });
      }

      if (req.predicate?.tags[0].value === 'commit/gitiles/5678') {
        return SearchBuildsResponse.fromPartial({
          builds: [
            createMockBuild('00002'),
            createMockBuild('00003'),
            createMockBuild('00004', ['00002']),
          ],
        });
      }

      throw new Error('unexpected request');
    });
    render(
      <FakeContextProvider>
        <RelatedBuildsDisplay build={selfBuild} />
      </FakeContextProvider>,
    );
    expect(searchBuildsMock).toHaveBeenCalledTimes(4);
    expect(searchBuildsMock).toHaveBeenCalledWith(
      SearchBuildsRequest.fromPartial({
        mask: {
          fields: PARTIAL_BUILD_FIELD_MASK,
        },
        pageSize: 1000,
        predicate: {
          tags: [
            {
              key: 'buildset',
              value: 'commit/gitiles/1234',
            },
          ],
        },
      }),
    );
    expect(searchBuildsMock).toHaveBeenCalledWith(
      SearchBuildsRequest.fromPartial({
        mask: {
          fields: PARTIAL_BUILD_FIELD_MASK,
        },
        pageSize: 1000,
        predicate: {
          tags: [
            {
              key: 'buildset',
              value: 'commit/gitiles/1234',
            },
          ],
        },
      }),
    );
    expect(searchBuildsMock).toHaveBeenCalledWith(
      SearchBuildsRequest.fromPartial({
        mask: {
          fields: PARTIAL_BUILD_FIELD_MASK,
        },
        pageSize: 1000,
        predicate: {
          tags: [
            {
              key: 'buildset',
              value: 'commit/gitiles/5678',
            },
          ],
        },
      }),
    );

    await act(() => jest.advanceTimersByTimeAsync(1000));

    expect(relatedBuildsTableSpy).toHaveBeenLastCalledWith(
      {
        selfBuild: selfBuild,
        buildTree: new CategoryTree(
          [
            selfBuild,
            createMockBuild('00001'),
            createMockBuild('00002'),
            createMockBuild('00003'),
            createMockBuild('00004', ['00002']),
            createMockBuild('00005'),
          ].map((b) => [[...b.ancestorIds, b.id], b]),
        ),
        showLoadingBar: false,
      },
      expect.anything(),
    );
  });

  it('one query is loading', async () => {
    const selfBuild = Build.fromPartial({
      id: '00004',
      builder: {
        project: 'proj',
        bucket: 'bucket',
        builder: 'builder',
      },
      ancestorIds: ['00002'],
      tags: [
        { key: 'otherkey', value: 'othervalue' },
        { key: 'buildset', value: 'commit/git/1234' },
        { key: 'buildset', value: 'commit/gitiles/1234' },
        { key: 'buildset', value: 'commit/git/5678' },
        { key: 'buildset', value: 'commit/gitiles/5678' },
      ],
    }) as OutputBuild;

    searchBuildsMock.mockImplementation(async (req) => {
      if (
        req.predicate?.build?.startBuildId === '00002' &&
        req.predicate?.build?.endBuildId === '00002'
      ) {
        return SearchBuildsResponse.fromPartial({
          builds: [createMockBuild('00002')],
        });
      }

      return NEVER_PROMISE;
    });
    render(
      <FakeContextProvider>
        <RelatedBuildsDisplay build={selfBuild} />
      </FakeContextProvider>,
    );

    await act(() => jest.advanceTimersByTimeAsync(1000));

    expect(relatedBuildsTableSpy).toHaveBeenLastCalledWith(
      {
        selfBuild: selfBuild,
        buildTree: new CategoryTree(
          [selfBuild, createMockBuild('00002')].map((b) => [
            [...b.ancestorIds, b.id],
            b,
          ]),
        ),
        showLoadingBar: true,
      },
      expect.anything(),
    );
  });
});
