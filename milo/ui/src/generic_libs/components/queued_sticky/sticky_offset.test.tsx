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
import ResizeObserver from 'resize-observer-polyfill';

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { QueuedStickyScrollingBase } from './queued_sticky_scrolling_based';
import { Sticky } from './sticky';
import { StickyOffset } from './sticky_offset';

global.ResizeObserver = ResizeObserver;

describe('<StickyOffset />', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('should populate style rules correctly', async () => {
    render(
      <FakeContextProvider>
        <QueuedStickyScrollingBase>
          <Sticky top>
            <div data-testid="line-1">line 1</div>
          </Sticky>
          <StickyOffset>
            <Sticky top>
              <div data-testid="line-2">line 2</div>
            </Sticky>
            <StickyOffset>
              <Sticky top>
                <div data-testid="line-3">line 3</div>
              </Sticky>
            </StickyOffset>
          </StickyOffset>
        </QueuedStickyScrollingBase>
      </FakeContextProvider>,
    );

    await jest.runAllTimersAsync();

    const sticky1 = screen.getByTestId('line-1').parentElement;
    const sticky2 = screen.getByTestId('line-2').parentElement;
    const sticky3 = screen.getByTestId('line-3').parentElement;
    expect(sticky1).toHaveStyleRule('top', 'var(--accumulated-top)');
    expect(sticky2).toHaveStyleRule('top', 'var(--accumulated-top)');
    expect(sticky3).toHaveStyleRule('top', 'var(--accumulated-top)');

    const offset1 = sticky2?.parentElement;
    // New offsets are added.
    //
    // TODO: jest-dom is not able to calculate sizes. Add an integration test to
    // ensure sizes are correct.
    expect(offset1).toHaveStyle({
      '--accumulated-top-1': 'calc(var(--accumulated-top-0) + 0px)',
      '--accumulated-right-1': 'calc(var(--accumulated-right-0) + 0px)',
      '--accumulated-bottom-1': 'calc(var(--accumulated-bottom-0) + 0px)',
      '--accumulated-left-1': 'calc(var(--accumulated-left-0) + 0px)',
    });
    // Stable CSS variables should be updated to point to the corresponding
    // variables in the current layer.
    expect(offset1).toHaveStyle({
      '--accumulated-top': 'var(--accumulated-top-1)',
      '--accumulated-right': 'var(--accumulated-right-1)',
      '--accumulated-bottom': 'var(--accumulated-bottom-1)',
      '--accumulated-left': 'var(--accumulated-left-1)',
    });

    const offset2 = sticky3?.parentElement;
    // New offsets are added.
    //
    // TODO: jest-dom is not able to calculate sizes. Add an integration test to
    // ensure sizes are correct.
    expect(offset2).toHaveStyle({
      '--accumulated-top-2': 'calc(var(--accumulated-top-1) + 0px)',
      '--accumulated-right-2': 'calc(var(--accumulated-right-1) + 0px)',
      '--accumulated-bottom-2': 'calc(var(--accumulated-bottom-1) + 0px)',
      '--accumulated-left-2': 'calc(var(--accumulated-left-1) + 0px)',
    });
    // Stable CSS variables should be updated to point to the corresponding
    // variables in the current layer.
    expect(offset2).toHaveStyle({
      '--accumulated-top': 'var(--accumulated-top-2)',
      '--accumulated-right': 'var(--accumulated-right-2)',
      '--accumulated-bottom': 'var(--accumulated-bottom-2)',
      '--accumulated-left': 'var(--accumulated-left-2)',
    });
  });
});
