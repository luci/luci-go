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

import { act, render, screen } from '@testing-library/react';

import { usePrpcQuery } from '@/common/hooks/legacy_prpc_query';
import { MiloInternal } from '@/common/services/milo_internal';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { CustomBugLink } from './custom_bug_link';

jest.mock('@/common/hooks/legacy_prpc_query', () => {
  return createSelectiveSpiesFromModule<
    typeof import('@/common/hooks/legacy_prpc_query')
  >('@/common/hooks/legacy_prpc_query', ['usePrpcQuery']);
});

describe('CustomBugLink', () => {
  let usePrpcQueryMock: jest.MockedFunction<typeof usePrpcQuery>;

  beforeEach(() => {
    jest.useFakeTimers();
    usePrpcQueryMock = jest.mocked(usePrpcQuery);
    jest.spyOn(MiloInternal.prototype, 'getProjectCfg').mockResolvedValue({
      bugUrlTemplate:
        'https://b.corp.google.com/createIssue?description=builder%3A{{{milo_builder_url}}}build%3A{{{milo_build_url}}}',
    });
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  test('e2e', async () => {
    const { rerender } = render(
      <FakeContextProvider>
        <CustomBugLink project="proj" />
      </FakeContextProvider>,
    );
    await act(() => jest.runAllTimersAsync());

    // The query is sent even when `build` is not yet populated.
    expect(usePrpcQueryMock).toHaveBeenCalledWith(
      expect.objectContaining({
        host: '',
        insecure: location.protocol === 'http:',
        Service: MiloInternal,
        method: 'getProjectCfg',
        request: { project: 'proj' },
      }),
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
