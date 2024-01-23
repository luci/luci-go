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

import { cleanup, render, screen } from '@testing-library/react';
import { act } from 'react-dom/test-utils';

import { Build } from '@/proto/go.chromium.org/luci/buildbucket/proto/build.pb';
import {
  BuildsClientImpl,
  SearchBuildsRequest,
  SearchBuildsResponse,
} from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';
import {
  Status,
  StringPair,
} from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';
import { NEVER_PROMISE } from '@/testing_tools/utils';

import { RelatedBuildTable } from './related_build_table';
import {
  RELATED_BUILDS_FIELD_MASK,
  RelatedBuildsDisplay,
} from './related_builds_display';

function createMockBuild(id: string): Build {
  return Build.fromPartial({
    id,
    builder: {
      bucket: 'buck',
      builder: 'builder',
      project: 'proj',
    },
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

  it('no buildset', async () => {
    render(
      <FakeContextProvider>
        <RelatedBuildsDisplay
          buildTags={[{ key: 'otherkey', value: 'othervalue' }]}
        />
      </FakeContextProvider>,
    );
    expect(searchBuildsMock).not.toHaveBeenCalled();

    expect(
      screen.getByText('No other builds found with the same buildset'),
    ).toBeInTheDocument();
  });

  it('can dedupe builds', async () => {
    searchBuildsMock.mockImplementation(async (req) => {
      if (req.predicate?.tags[0].value === 'commit/gitiles/1234') {
        return SearchBuildsResponse.fromPartial({
          builds: [
            createMockBuild('00001'),
            createMockBuild('00002'),
          ] as ReadonlyArray<Build>,
        });
      }

      return SearchBuildsResponse.fromPartial({
        builds: [
          createMockBuild('00002'),
          createMockBuild('00003'),
        ] as ReadonlyArray<Build>,
      });
    });
    render(
      <FakeContextProvider>
        <RelatedBuildsDisplay
          buildTags={[
            { key: 'otherkey', value: 'othervalue' },
            { key: 'buildset', value: 'commit/git/1234' },
            { key: 'buildset', value: 'commit/gitiles/1234' },
            { key: 'buildset', value: 'commit/git/5678' },
            { key: 'buildset', value: 'commit/gitiles/5678' },
          ]}
        />
      </FakeContextProvider>,
    );
    expect(searchBuildsMock).toHaveBeenCalledTimes(2);
    expect(searchBuildsMock).toHaveBeenCalledWith(
      SearchBuildsRequest.fromPartial({
        fields: RELATED_BUILDS_FIELD_MASK,
        pageSize: 1000,
        predicate: {
          tags: [
            {
              key: 'buildset',
              value: 'commit/gitiles/1234',
            },
          ] as ReadonlyArray<StringPair>,
        },
      }),
    );
    expect(searchBuildsMock).toHaveBeenCalledWith(
      SearchBuildsRequest.fromPartial({
        fields: RELATED_BUILDS_FIELD_MASK,
        pageSize: 1000,
        predicate: {
          tags: [
            {
              key: 'buildset',
              value: 'commit/gitiles/5678',
            },
          ] as ReadonlyArray<StringPair>,
        },
      }),
    );

    await act(() => jest.runAllTimersAsync());
    await act(() => jest.runAllTimersAsync());

    expect(relatedBuildsTableSpy).toHaveBeenCalledWith(
      {
        relatedBuilds: [
          createMockBuild('00001'),
          createMockBuild('00002'),
          createMockBuild('00003'),
        ],
        showLoadingBar: false,
      },
      expect.anything(),
    );
  });

  it('one query is loading', async () => {
    searchBuildsMock.mockImplementation(async (req) => {
      if (req.predicate?.tags[0].value === 'commit/git/1234') {
        return SearchBuildsResponse.fromPartial({
          builds: [
            createMockBuild('00001'),
            createMockBuild('00002'),
          ] as ReadonlyArray<Build>,
        });
      }

      return NEVER_PROMISE;
    });
    render(
      <FakeContextProvider>
        <RelatedBuildsDisplay
          buildTags={[
            { key: 'otherkey', value: 'othervalue' },
            { key: 'buildset', value: 'commit/git/1234' },
            { key: 'buildset', value: 'commit/gitiles/1234' },
            { key: 'buildset', value: 'commit/git/5678' },
            { key: 'buildset', value: 'commit/gitiles/5678' },
          ]}
        />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());

    expect(relatedBuildsTableSpy).toHaveBeenCalledWith(
      {
        relatedBuilds: [],
        showLoadingBar: true,
      },
      expect.anything(),
    );
  });
});
