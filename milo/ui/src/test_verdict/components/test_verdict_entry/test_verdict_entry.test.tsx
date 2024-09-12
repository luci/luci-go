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

import { NEVER_PROMISE } from '@/common/constants/utils';
import { ResultDBClientImpl } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { TestVerdictEntry } from './test_verdict_entry';

describe('<TestVerdictEntry />', () => {
  beforeEach(() => {
    jest.useFakeTimers();
    jest
      .spyOn(ResultDBClientImpl.prototype, 'BatchGetTestVariants')
      .mockResolvedValue(NEVER_PROMISE);
  });

  afterEach(() => {
    jest.restoreAllMocks();
    jest.useRealTimers();
  });

  it('should update expansion state when default value is updated', async () => {
    const { rerender } = render(
      <FakeContextProvider>
        <TestVerdictEntry
          project="proj"
          testId="test-id"
          variantHash="v-hash"
          invocationId="inv-id"
          defaultExpanded={true}
        />
      </FakeContextProvider>,
    );
    await jest.runOnlyPendingTimersAsync();
    expect(screen.queryByRole('progressbar')).toBeInTheDocument();

    rerender(
      <FakeContextProvider>
        <TestVerdictEntry
          project="proj"
          testId="test-id"
          variantHash="v-hash"
          invocationId="inv-id"
          defaultExpanded={false}
        />
      </FakeContextProvider>,
    );
    await jest.runOnlyPendingTimersAsync();
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();

    rerender(
      <FakeContextProvider>
        <TestVerdictEntry
          project="proj"
          testId="test-id"
          variantHash="v-hash"
          invocationId="inv-id"
          defaultExpanded={true}
        />
      </FakeContextProvider>,
    );
    await jest.runOnlyPendingTimersAsync();
    expect(screen.queryByRole('progressbar')).toBeInTheDocument();
  });
});
