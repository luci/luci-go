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

import { NEVER_PROMISE } from '@/common/constants/utils';
import { ResultDBClientImpl } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { InvocationIdBar } from './invocation_id_bar';

describe('<InvocationIdBar />', () => {
  let getInvocationMock: jest.SpiedFunction<
    ResultDBClientImpl['GetInvocation']
  >;

  beforeEach(() => {
    getInvocationMock = jest
      .spyOn(ResultDBClientImpl.prototype, 'GetInvocation')
      .mockResolvedValue(NEVER_PROMISE);
  });

  afterEach(() => {
    cleanup();
    getInvocationMock.mockRestore();
  });

  it('should display build page link for build invocations', () => {
    render(
      <FakeContextProvider>
        <InvocationIdBar invName="invocations/build-1234" />
      </FakeContextProvider>,
    );
    expect(screen.getByText('build')).toHaveAttribute('href', '/ui/b/1234');
  });

  it('should display swarming task page link for task invocations', () => {
    render(
      <FakeContextProvider>
        <InvocationIdBar invName="invocations/task-chromium-swarm-dev.appspot.com-6795acb84c9cff11" />
      </FakeContextProvider>,
    );
    expect(screen.getByText('task')).toHaveAttribute(
      'href',
      'https://chromium-swarm-dev.appspot.com/task?id=6795acb84c9cff11&o=true&w=true',
    );
  });

  it('should not display additional link for other invocations', () => {
    render(
      <FakeContextProvider>
        <InvocationIdBar invName="invocations/inv-1234" />
      </FakeContextProvider>,
    );
    expect(screen.queryByText('build')).not.toBeInTheDocument();
    expect(screen.queryByText('task')).not.toBeInTheDocument();
  });
});
