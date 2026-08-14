// Copyright 2026 The LUCI Authors.
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
import { DateTime } from 'luxon';

import { LONG_TIME_FORMAT } from '@/common/tools/time_utils';
import { OutputTestVerdict } from '@/common/types/verdict';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import {
  InvocationProvider,
  TestVariantProvider,
} from '@/test_investigation/context';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { TestInfoContext, TestInfoContextValue } from '../context/context';

import { RecommendationsSection } from './recommendations_section';

describe('<RecommendationsSection />', () => {
  const defaultTestInfoContextValue: TestInfoContextValue = {
    testVariantBranch: null,
    formattedCls: [],
    associatedBugs: [],
    isLoadingAssociatedBugs: false,
    isDrawerOpen: false,
    onToggleDrawer: jest.fn(),
  };

  const renderComponent = (invocation: Invocation) => {
    const testVariant = TestVariant.fromPartial({
      testId: 'test-id',
    });

    return render(
      <FakeContextProvider>
        <InvocationProvider
          project="test-project"
          invocation={invocation}
          rawInvocationId="inv-123"
          isLegacyInvocation={true}
        >
          <TestVariantProvider
            testVariant={testVariant as OutputTestVerdict}
            displayStatusString="failed"
          >
            <TestInfoContext.Provider value={defaultTestInfoContextValue}>
              <RecommendationsSection expanded={true} setExpanded={jest.fn()} />
            </TestInfoContext.Provider>
          </TestVariantProvider>
        </InvocationProvider>
      </FakeContextProvider>,
    );
  };

  it('renders timestamp when finalizeTime is provided', () => {
    const finalizeTime = '2026-08-04T19:07:00Z';
    const invocation = Invocation.fromPartial({
      finalizeTime,
    });

    renderComponent(invocation);

    const expectedDateStr =
      DateTime.fromISO(finalizeTime).toFormat(LONG_TIME_FORMAT);
    expect(screen.getByText(expectedDateStr)).toBeInTheDocument();
  });

  it('renders N/A when finalizeTime is not provided', () => {
    const invocation = Invocation.fromPartial({});

    renderComponent(invocation);

    expect(screen.getByText('Last update: N/A')).toBeInTheDocument();
  });
});
