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

import { render, screen, waitFor } from '@testing-library/react';

import {
  FakeTestVerdictContextProvider,
  placeholderVerdict,
} from '../testing_tools/fake_context';

import { VerdictInfo } from './verdict_info';

describe('<VerdictInfo />', () => {
  it('given a verdict, should display verdict data', async () => {
    render(
      <FakeTestVerdictContextProvider>
        <VerdictInfo />
      </FakeTestVerdictContextProvider>,
    );
    waitFor(() => {
      expect(
        screen.getByText(
          'build_target:' + placeholderVerdict.variant!.def['build_target'],
        ),
      ).toBeInTheDocument();
      expect(screen.getByText('Test history')).toBeInTheDocument();
      expect(screen.getByText('History')).toBeInTheDocument();
    });
  });
});
