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

import { act, fireEvent, render, screen } from '@testing-library/react';

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { TestAggregationDrawer } from './test_aggregation_drawer';

jest.mock(
  '@/test_investigation/components/test_aggregation_viewer/test_aggregation_viewer',
  () => ({
    TestAggregationViewer: ({ autoLocate }: { autoLocate?: boolean }) => (
      <div
        data-testid="mock-test-aggregation-viewer"
        data-is-open={String(autoLocate)}
      />
    ),
  }),
);

jest.mock('@/test_investigation/context', () => ({
  useInvocation: () => ({ name: 'invocations/test' }),
  useTestVariant: () => ({ testId: 'test_id' }),
}));

describe('TestAggregationDrawer', () => {
  it('renders the drawer when isOpen is true', () => {
    render(
      <FakeContextProvider>
        <TestAggregationDrawer isOpen={true} onClose={jest.fn()} />
      </FakeContextProvider>,
    );
    expect(
      screen.getByTestId('mock-test-aggregation-viewer'),
    ).toBeInTheDocument();
  });

  it('calls onClose when backdrop is clicked', () => {
    const onClose = jest.fn();
    render(
      <FakeContextProvider>
        <TestAggregationDrawer isOpen={true} onClose={onClose} />
      </FakeContextProvider>,
    );

    // The backdrop is rendered as the previous sibling of the drawer.
    const backdrop = document.querySelector('.MuiBackdrop-root');
    expect(backdrop).toBeInTheDocument();
    fireEvent.click(backdrop!);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('resizes the drawer within limits on mouse drag', () => {
    // Mock window innerWidth for consistent max width
    Object.defineProperty(window, 'innerWidth', {
      writable: true,
      configurable: true,
      value: 2000,
    }); // max width 50vw = 1000px

    render(
      <FakeContextProvider>
        <TestAggregationDrawer isOpen={true} onClose={jest.fn()} />
      </FakeContextProvider>,
    );

    // To test resize, we need to trigger mousedown on the DragHandle (the div over the viewer)
    // The DragHandle has a col-resize cursor style.
    const viewer = screen.getByTestId('mock-test-aggregation-viewer');
    const dragHandle = viewer.nextElementSibling as HTMLElement;
    expect(dragHandle).toBeInTheDocument();

    // The drawer root normally has the MuiDrawer-paper width style
    const paper = document.querySelector('.MuiDrawer-paper') as HTMLElement;
    expect(paper).toBeInTheDocument();

    // Check initial width (500)
    expect(paper.style.width).toBe('500px');

    // Mousedown
    act(() => {
      fireEvent.mouseDown(dragHandle);
    });

    // Mousemove within limits
    act(() => {
      fireEvent.mouseMove(document, { clientX: 600 });
    });
    expect(paper.style.width).toBe('600px');

    // Mousemove below MIN_DRAWER_WIDTH (500px)
    act(() => {
      fireEvent.mouseMove(document, { clientX: 400 });
    });
    expect(paper.style.width).toBe('500px'); // Clamped

    // Mousemove above max width (50vw = 1000px)
    act(() => {
      fireEvent.mouseMove(document, { clientX: 1200 });
    });
    expect(paper.style.width).toBe('1000px'); // Clamped

    // Mouseup finishes dragging
    act(() => {
      fireEvent.mouseUp(document);
    });

    // Further mouse moves should do nothing
    act(() => {
      fireEvent.mouseMove(document, { clientX: 600 });
    });
    expect(paper.style.width).toBe('1000px');
  });
});
