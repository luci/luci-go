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

import { render, screen } from '@testing-library/react';

import {
  GeneralState,
  Status,
} from '@/proto/go.chromium.org/luci/tree_status/proto/v1/tree_status.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { TreeStatusTable } from './tree_status_table';

const status = [
  Status.fromPartial({
    name: 'ignored',
    generalState: GeneralState.OPEN,
    message: 'Tree is ready for commits',
    createTime: '2024-01-22T00:29:47.998Z',
    createUser: 'test@example.com',
  }),
];

it('displays a status update', async () => {
  render(
    <FakeContextProvider>
      <TreeStatusTable status={status} />
    </FakeContextProvider>,
  );
  expect(screen.getByText('Open')).toBeInTheDocument();
  expect(screen.getByText('Tree is ready for commits')).toBeInTheDocument();
});
