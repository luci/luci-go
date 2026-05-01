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

import { fireEvent, render, screen } from '@testing-library/react';

import '@testing-library/jest-dom';

import { WidgetPortalContext } from '@/crystal_ball/context';

import { WidgetSidePanel } from './widget_side_panel';

describe('WidgetSidePanel', () => {
  it('should render content inside the portal target', () => {
    const target = document.createElement('div');
    document.body.appendChild(target);

    const onClose = jest.fn();

    render(
      <WidgetPortalContext.Provider
        value={{
          target,
          isOpen: true,
          setOpen: jest.fn(),
          isFolded: false,
          fold: jest.fn(),
          expand: jest.fn(),
        }}
      >
        <WidgetSidePanel title="Test Title" onClose={onClose}>
          <div>Test Children</div>
        </WidgetSidePanel>
      </WidgetPortalContext.Provider>,
    );

    // Verify content is rendered
    expect(screen.getByText('Test Title')).toBeInTheDocument();
    expect(screen.getByText('Test Children')).toBeInTheDocument();

    // Verify close interaction
    const closeBtn = screen.getByTitle('Close panel');
    fireEvent.click(closeBtn);
    expect(onClose).toHaveBeenCalledTimes(1);

    // Cleanup
    document.body.removeChild(target);
  });

  it('should return null and not render if no portal target is provided', () => {
    const { container } = render(
      <WidgetPortalContext.Provider
        value={{
          target: null,
          isOpen: false,
          setOpen: jest.fn(),
          isFolded: false,
          fold: jest.fn(),
          expand: jest.fn(),
        }}
      >
        <WidgetSidePanel title="Test Title" onClose={jest.fn()}>
          <div>Test Children</div>
        </WidgetSidePanel>
      </WidgetPortalContext.Provider>,
    );
    expect(container).toBeEmptyDOMElement();
  });
});
