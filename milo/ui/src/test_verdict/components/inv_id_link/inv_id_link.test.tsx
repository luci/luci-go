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

import { InvIdLink } from './inv_id_link';

describe('<InvIdLink />', () => {
  afterEach(() => {
    cleanup();
  });

  it('works with build invocation', () => {
    render(
      <FakeContextProvider>
        <InvIdLink invId="build-1234" />
      </FakeContextProvider>,
    );
    expect(screen.getByRole('link')).toHaveAttribute('href', '/ui/b/1234');
  });

  it('works with swarming invocation', () => {
    render(
      <FakeContextProvider>
        <InvIdLink invId="task-chromium-swarm-dev.appspot.com-a0134d" />
      </FakeContextProvider>,
    );
    expect(screen.getByRole('link')).toHaveAttribute(
      'href',
      'https://chromium-swarm-dev.appspot.com/task?id=a0134d&o=true&w=true',
    );
  });

  it('works with regular invocation', () => {
    render(
      <FakeContextProvider>
        <InvIdLink invId="a-regular-invocation-id" />
      </FakeContextProvider>,
    );
    expect(screen.getByRole('link')).toHaveAttribute(
      'href',
      '/ui/inv/a-regular-invocation-id',
    );
  });
});
