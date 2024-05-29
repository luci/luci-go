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
import { act } from 'react';

import {
  Invocation,
  Invocation_State,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { ResultDBClientImpl } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { DetailsTab } from './details_tab';

describe('<DetailsTab />', () => {
  let getInvocationMock: jest.SpiedFunction<
    ResultDBClientImpl['GetInvocation']
  >;

  beforeEach(() => {
    jest.useFakeTimers();
    getInvocationMock = jest.spyOn(
      ResultDBClientImpl.prototype,
      'GetInvocation',
    );
  });

  afterEach(() => {
    cleanup();
    jest.useRealTimers();
    getInvocationMock.mockRestore();
  });

  it('can handle invocations with some missing timestamp', async () => {
    getInvocationMock.mockResolvedValue(
      Invocation.fromPartial({
        state: Invocation_State.ACTIVE,
        createTime: '2002-02-02',
        finalizeStartTime: undefined,
        finalizeTime: undefined,
        deadline: '2002-02-03',
      }),
    );
    render(
      <FakeContextProvider
        mountedPath="inv/:invId"
        routerOptions={{ initialEntries: ['/inv/inv-id'] }}
      >
        <DetailsTab />
      </FakeContextProvider>,
    );
    await act(() => jest.runAllTimersAsync());
    expect(screen.getByText('Created At:').nextSibling).toHaveTextContent(
      '00:00:00 Sat, Feb 02 2002 UTC',
    );
    expect(screen.getByText('Finalizing At:').nextSibling).toHaveTextContent(
      'N/A',
    );
    expect(screen.getByText('Finalized At:').nextSibling).toHaveTextContent(
      'N/A',
    );
    expect(screen.getByText('Deadline:').nextSibling).toHaveTextContent(
      '00:00:00 Sun, Feb 03 2002 UTC',
    );
  });

  it('general invocation properties are displayed', async () => {
    getInvocationMock.mockResolvedValue(
      Invocation.fromPartial({
        state: Invocation_State.ACTIVE,
        realm: 'chromium:try',
        createdBy: 'project:chromium',
        baselineId: 'try:my-builder',
        isExportRoot: true,
      }),
    );
    render(
      <FakeContextProvider
        mountedPath="inv/:invId"
        routerOptions={{ initialEntries: ['/inv/inv-id'] }}
      >
        <DetailsTab />
      </FakeContextProvider>,
    );
    await act(() => jest.runAllTimersAsync());
    expect(screen.getByText('State:').nextSibling).toHaveTextContent('ACTIVE');
    expect(screen.getByText('Realm:').nextSibling).toHaveTextContent(
      'chromium:try',
    );
    expect(screen.getByText('Created By:').nextSibling).toHaveTextContent(
      'project:chromium',
    );
    expect(screen.getByText('Baseline:').nextSibling).toHaveTextContent(
      'try:my-builder',
    );
    expect(screen.getByText('Is Export Root:').nextSibling).toHaveTextContent(
      'True',
    );
  });
});
