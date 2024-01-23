// Copyright 2022 The LUCI Authors.
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

import { act, cleanup, render } from '@testing-library/react';

import {
  ListBuildersResponse,
  MiloInternalClientImpl,
} from '@/proto/go.chromium.org/luci/milo/proto/v1/rpc.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { BuilderList } from './builder_list';
import { BuilderListDisplay } from './builder_list_display';

jest.mock('./builder_list_display', () => {
  return createSelectiveSpiesFromModule<
    typeof import('./builder_list_display')
  >('./builder_list_display', ['BuilderListDisplay']);
});

const builderPages: { [pageToken: string]: ListBuildersResponse } = {
  '': ListBuildersResponse.fromPartial({
    builders: Object.freeze([
      {
        id: { project: 'proj1', bucket: 'bucket1', builder: 'builder1' },
      },
      {
        id: { project: 'proj1', bucket: 'bucket1', builder: 'builder2' },
      },
    ]),
    nextPageToken: 'page2',
  }),
  page2: ListBuildersResponse.fromPartial({
    builders: Object.freeze([
      {
        id: { project: 'proj1', bucket: 'bucket2', builder: 'builder1' },
      },
      {
        id: { project: 'proj1', bucket: 'bucket2', builder: 'builder2' },
      },
    ]),
    nextPageToken: 'page3',
  }),
  page3: ListBuildersResponse.fromPartial({
    builders: Object.freeze([
      {
        id: { project: 'proj2', bucket: 'bucket1', builder: 'builder1' },
      },
      {
        id: { project: 'proj2', bucket: 'bucket1', builder: 'builder2' },
      },
    ]),
  }),
};

describe('BuilderList', () => {
  let listBuilderMock: jest.SpyInstance;
  let builderListDisplaySpy: jest.MockedFunctionDeep<typeof BuilderListDisplay>;

  beforeEach(() => {
    jest.useFakeTimers();
    listBuilderMock = jest
      .spyOn(MiloInternalClientImpl.prototype, 'ListBuilders')
      .mockImplementation(async ({ pageToken }) => {
        return builderPages[pageToken || ''];
      });
    builderListDisplaySpy = jest.mocked(BuilderListDisplay);
  });
  afterEach(() => {
    cleanup();
    jest.useRealTimers();
  });

  it('e2e', async () => {
    const { rerender } = render(
      <FakeContextProvider>
        <BuilderList searchQuery="" />
      </FakeContextProvider>,
    );

    await act(() => jest.runAllTimersAsync());
    await act(() => jest.runAllTimersAsync());
    await act(() => jest.runAllTimersAsync());

    // All the pages should've been be loaded.
    expect(listBuilderMock).toHaveBeenCalledTimes(3);
    expect(builderListDisplaySpy).toHaveBeenLastCalledWith(
      {
        groupedBuilders: {
          'proj1/bucket1': [
            { project: 'proj1', bucket: 'bucket1', builder: 'builder1' },
            { project: 'proj1', bucket: 'bucket1', builder: 'builder2' },
          ],
          'proj1/bucket2': [
            { project: 'proj1', bucket: 'bucket2', builder: 'builder1' },
            { project: 'proj1', bucket: 'bucket2', builder: 'builder2' },
          ],
          'proj2/bucket1': [
            { project: 'proj2', bucket: 'bucket1', builder: 'builder1' },
            { project: 'proj2', bucket: 'bucket1', builder: 'builder2' },
          ],
        },
        isLoading: false,
      },
      expect.anything(),
    );

    // Filter builder.
    rerender(
      <FakeContextProvider>
        <BuilderList searchQuery="builder2" />
      </FakeContextProvider>,
    );
    // Do not trigger more list builder calls.
    expect(listBuilderMock).toHaveBeenCalledTimes(3);
    // Builders are filtered correctly.
    expect(builderListDisplaySpy).toHaveBeenLastCalledWith(
      {
        groupedBuilders: {
          'proj1/bucket1': [
            { project: 'proj1', bucket: 'bucket1', builder: 'builder2' },
          ],
          'proj1/bucket2': [
            { project: 'proj1', bucket: 'bucket2', builder: 'builder2' },
          ],
          'proj2/bucket1': [
            { project: 'proj2', bucket: 'bucket1', builder: 'builder2' },
          ],
        },
        isLoading: false,
      },
      expect.anything(),
    );

    // Fuzzy search builder.
    rerender(
      <FakeContextProvider>
        <BuilderList searchQuery="CkEt1/bU Oj2" />
      </FakeContextProvider>,
    );
    // Do not trigger more list builder calls.
    expect(listBuilderMock).toHaveBeenCalledTimes(3);
    // Builders are filtered correctly.
    expect(builderListDisplaySpy).toHaveBeenLastCalledWith(
      {
        groupedBuilders: {
          'proj2/bucket1': [
            { project: 'proj2', bucket: 'bucket1', builder: 'builder1' },
            { project: 'proj2', bucket: 'bucket1', builder: 'builder2' },
          ],
        },
        isLoading: false,
      },
      expect.anything(),
    );
  });
});
