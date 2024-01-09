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

import { configuredTrees } from '@/monitoring/util/config';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { FileBugDialog } from './file_bug_dialog';

it('renders', async () => {
  const onClose = jest.fn(() => {});
  render(
    <FakeContextProvider>
      <FileBugDialog
        alerts={[]}
        tree={configuredTrees[0]}
        open={true}
        onClose={onClose}
      />
    </FakeContextProvider>,
  );
  expect(screen.getByText('File a new bug')).toBeInTheDocument();
});
