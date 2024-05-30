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
  it('collapsed', () => {
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

    expect(screen.findByTestId('ChevronRightIcon')).not.toBeNull();
    expect(screen.queryByText('Header')).not.toBeNull();
    expect(screen.queryByText('Content')).toBeNull();
  });

  it('expanded', () => {
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

    expect(screen.getByTestId('ExpandMoreIcon')).not.toBeNull();
    expect(screen.queryByText('Header')).not.toBeNull();
    expect(screen.queryByText('Content')).not.toBeNull();
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
