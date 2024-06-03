// Copyright 2022 The LUCI Authors.
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

import {
  ExpandableEntry,
  ExpandableEntryBody,
  ExpandableEntryHeader,
} from './expandable_entry';

describe('<ExpandableEntry />', () => {
  it('can collapse', () => {
    render(
      <ExpandableEntry expanded={false}>
        <ExpandableEntryHeader onToggle={() => {}}>
          <span>Header</span>
        </ExpandableEntryHeader>
        <ExpandableEntryBody>
          <span>Content</span>
        </ExpandableEntryBody>
      </ExpandableEntry>,
    );

    expect(screen.getByTestId('ChevronRightIcon')).toBeVisible();
    expect(screen.queryByText('Header')).toBeVisible();

    // Either the content is not in DOM or its invisible.
    expect(
      screen.queryByText('Content') || document.createElement('div'),
    ).not.toBeVisible();
  });

  it('can expand', () => {
    render(
      <ExpandableEntry expanded={true}>
        <ExpandableEntryHeader onToggle={() => {}}>
          <span>Header</span>
        </ExpandableEntryHeader>
        <ExpandableEntryBody>
          <span>Content</span>
        </ExpandableEntryBody>
      </ExpandableEntry>,
    );

    expect(screen.getByTestId('ExpandMoreIcon')).toBeVisible();
    expect(screen.queryByText('Header')).toBeVisible();
    expect(screen.queryByText('Content')).toBeVisible();
  });

  it('should keep content in DOM once expanded in "was-expanded" mode', () => {
    const { rerender } = render(
      <ExpandableEntry expanded={false}>
        <ExpandableEntryHeader onToggle={() => {}}>
          <span>Header</span>
        </ExpandableEntryHeader>
        <ExpandableEntryBody renderChildren="was-expanded">
          <span>Content</span>
        </ExpandableEntryBody>
      </ExpandableEntry>,
    );

    expect(screen.queryByText('Content')).not.toBeInTheDocument();

    rerender(
      <ExpandableEntry expanded={true}>
        <ExpandableEntryHeader onToggle={() => {}}>
          <span>Header</span>
        </ExpandableEntryHeader>
        <ExpandableEntryBody renderChildren="was-expanded">
          <span>Content</span>
        </ExpandableEntryBody>
      </ExpandableEntry>,
    );

    expect(screen.queryByText('Content')).toBeVisible();

    rerender(
      <ExpandableEntry expanded={false}>
        <ExpandableEntryHeader onToggle={() => {}}>
          <span>Header</span>
        </ExpandableEntryHeader>
        <ExpandableEntryBody renderChildren="was-expanded">
          <span>Content</span>
        </ExpandableEntryBody>
      </ExpandableEntry>,
    );

    expect(screen.queryByText('Content')).toBeInTheDocument();
    expect(screen.queryByText('Content')).not.toBeVisible();
  });

  it('should always keep content in DOM in "always" mode', () => {
    const { rerender } = render(
      <ExpandableEntry expanded={false}>
        <ExpandableEntryHeader onToggle={() => {}}>
          <span>Header</span>
        </ExpandableEntryHeader>
        <ExpandableEntryBody renderChildren="always">
          <span>Content</span>
        </ExpandableEntryBody>
      </ExpandableEntry>,
    );

    expect(screen.queryByText('Content')).toBeInTheDocument();
    expect(screen.queryByText('Content')).not.toBeVisible();

    rerender(
      <ExpandableEntry expanded={true}>
        <ExpandableEntryHeader onToggle={() => {}}>
          <span>Header</span>
        </ExpandableEntryHeader>
        <ExpandableEntryBody renderChildren="always">
          <span>Content</span>
        </ExpandableEntryBody>
      </ExpandableEntry>,
    );

    expect(screen.queryByText('Content')).toBeVisible();
  });

  it('should remove content in DOM  only when expanded in "expanded" mode', () => {
    const { rerender } = render(
      <ExpandableEntry expanded={true}>
        <ExpandableEntryHeader onToggle={() => {}}>
          <span>Header</span>
        </ExpandableEntryHeader>
        <ExpandableEntryBody renderChildren="expanded">
          <span>Content</span>
        </ExpandableEntryBody>
      </ExpandableEntry>,
    );
    expect(screen.queryByText('Content')).toBeVisible();

    rerender(
      <ExpandableEntry expanded={false}>
        <ExpandableEntryHeader onToggle={() => {}}>
          <span>Header</span>
        </ExpandableEntryHeader>
        <ExpandableEntryBody renderChildren="expanded">
          <span>Content</span>
        </ExpandableEntryBody>
      </ExpandableEntry>,
    );

    expect(screen.queryByText('Content')).not.toBeInTheDocument();
  });

  it('onToggle should be called when the header is clicked', () => {
    const onToggleStub = jest.fn();
    const { rerender } = render(
      <ExpandableEntry expanded={false}>
        <ExpandableEntryHeader onToggle={onToggleStub}>
          <span>Header</span>
        </ExpandableEntryHeader>
        <ExpandableEntryBody>
          <span>Content</span>
        </ExpandableEntryBody>
      </ExpandableEntry>,
    );

    let headerIconEle = screen.getByTestId('ChevronRightIcon');
    const headerContentEle = screen.getByText('Header');

    expect(onToggleStub).toHaveBeenCalledTimes(0);
    fireEvent.click(headerIconEle);
    expect(onToggleStub).toHaveBeenCalledTimes(1);
    expect(onToggleStub).toHaveBeenLastCalledWith(true);

    // Clicking on the header the second time should not change the param passed
    // to onToggle.
    fireEvent.click(headerIconEle);
    expect(onToggleStub).toHaveBeenCalledTimes(2);
    expect(onToggleStub).toHaveBeenLastCalledWith(true);

    // Clicking on the header content should work as well.
    fireEvent.click(headerContentEle);
    expect(onToggleStub).toHaveBeenCalledTimes(3);
    expect(onToggleStub).toHaveBeenLastCalledWith(true);

    rerender(
      <ExpandableEntry expanded={true}>
        <ExpandableEntryHeader onToggle={onToggleStub}>
          <span>Header</span>
        </ExpandableEntryHeader>
        <ExpandableEntryBody>
          <span>Content</span>
        </ExpandableEntryBody>
      </ExpandableEntry>,
    );

    headerIconEle = screen.getByTestId('ExpandMoreIcon');

    // Updating the expanded prop is updated should change the param passed to
    // onToggle.
    expect(onToggleStub).toHaveBeenCalledTimes(3);
    fireEvent.click(headerIconEle);
    expect(onToggleStub).toHaveBeenCalledTimes(4);
    expect(onToggleStub).toHaveBeenLastCalledWith(false);
  });

  it('onToggle should not be called when the header is clicked when disabled', () => {
    const onToggleStub = jest.fn();
    render(
      <ExpandableEntry expanded={false}>
        <ExpandableEntryHeader disabled onToggle={onToggleStub}>
          <span>Header</span>
        </ExpandableEntryHeader>
        <ExpandableEntryBody>
          <span>Content</span>
        </ExpandableEntryBody>
      </ExpandableEntry>,
    );

    const headerIconEle = screen.getByTestId('ChevronRightIcon');
    const headerContentEle = screen.getByText('Header');

    expect(onToggleStub).not.toHaveBeenCalled();
    fireEvent.click(headerIconEle);
    expect(onToggleStub).not.toHaveBeenCalled();

    // Clicking on the header content should not trigger callback either.
    fireEvent.click(headerContentEle);
    expect(onToggleStub).not.toHaveBeenCalled();
  });
});
