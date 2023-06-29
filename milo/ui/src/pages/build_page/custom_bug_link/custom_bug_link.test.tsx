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

import '@testing-library/jest-dom';
import { UseQueryOptions } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';

import * as usePrpcQueryLib from '@/common/hooks/use_prpc_query';
import { MiloInternal, Project } from '@/common/services/milo_internal';

import { CustomBugLink } from './custom_bug_link';

jest.mock('@/common/hooks/use_prpc_query', () => {
  return createSelectiveMockFromModule<
    typeof import('@/common/hooks/use_prpc_query')
  >('@/common/hooks/use_prpc_query', ['usePrpcQuery']);
});

describe('CustomBugLink', () => {
  let usePrpcQueryMock: jest.MockedFunction<
    typeof usePrpcQueryLib.usePrpcQuery
  >;

  beforeEach(() => {
    usePrpcQueryMock = jest
      .mocked(usePrpcQueryLib.usePrpcQuery)
      .mockImplementation(({ options }) => {
        const opt = options as UseQueryOptions<Project>;
        // Use the actual `select` implementation so it can be tested as well.
        const selectFn = opt.select || ((d) => d);
        return {
          data: selectFn({
            bugUrlTemplate:
              'https://b.corp.google.com/createIssue?description=builder%3A{{{milo_builder_url}}}build%3A{{{milo_build_url}}}',
          }),
        } as ReturnType<typeof usePrpcQueryLib.usePrpcQuery>;
      });
  });

  test('e2e', () => {
    const { rerender } = render(<CustomBugLink project="proj" />);

    // The query is sent even when `build` is not yet populated.
    expect(usePrpcQueryMock).toHaveBeenCalledTimes(1);
    expect(usePrpcQueryMock.mock.lastCall?.[0]).toEqual({
      host: '',
      insecure: location.protocol === 'http:',
      Service: MiloInternal,
      method: 'getProjectCfg',
      request: { project: 'proj' },
      options: {
        // The actual implementation is used in mocks.
        // We only need to ensure that it exists here.
        select: expect.any(Function),
      },
    });
    expect(screen.queryByText('File a bug')).not.toBeInTheDocument();

    rerender(
      <CustomBugLink
        project="proj"
        build={{
          id: '1234',
          builder: { project: 'proj', bucket: 'bucket', builder: 'builder' },
        }}
      />
    );

    // The bug link is only rendered when `build` is populated.
    expect(screen.queryByText('File a bug')).toBeInTheDocument();
  });
});
