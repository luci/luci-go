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

import { WidgetContainer } from './widget_container';

describe('WidgetContainer', () => {
  it('renders title and children', () => {
    render(
      <WidgetContainer title="Test Widget">
        <div data-testid="child-content">Content</div>
      </WidgetContainer>,
    );

    expect(screen.getByText('Test Widget')).toBeInTheDocument();
    expect(screen.getByTestId('child-content')).toBeInTheDocument();
  });

  it('does not render action buttons if callbacks are not provided', () => {
    render(<WidgetContainer title="Test Widget">Content</WidgetContainer>);

    expect(
      screen.queryByLabelText(/Move Test Widget up/i),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByLabelText(/Move Test Widget down/i),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByLabelText(/Delete Test Widget/i),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByLabelText(/Edit title Test Widget/i),
    ).not.toBeInTheDocument();
  });

  it('renders and calls action buttons when provided', () => {
    const handleMoveUp = jest.fn();
    const handleMoveDown = jest.fn();

    render(
      <WidgetContainer
        title="Test Widget"
        onMoveUp={handleMoveUp}
        onMoveDown={handleMoveDown}
      >
        Content
      </WidgetContainer>,
    );

    const moveUpBtn = screen.getByLabelText(/Move Test Widget up/i);
    const moveDownBtn = screen.getByLabelText(/Move Test Widget down/i);

    fireEvent.click(moveUpBtn);
    expect(handleMoveUp).toHaveBeenCalledTimes(1);

    fireEvent.click(moveDownBtn);
    expect(handleMoveDown).toHaveBeenCalledTimes(1);
  });

  it('disables move buttons when props are passed', () => {
    render(
      <WidgetContainer
        title="Test Widget"
        onMoveUp={jest.fn()}
        onMoveDown={jest.fn()}
        disableMoveUp={true}
        disableMoveDown={true}
      >
        Content
      </WidgetContainer>,
    );

    expect(screen.getByLabelText(/Move Test Widget up/i)).toBeDisabled();
    expect(screen.getByLabelText(/Move Test Widget down/i)).toBeDisabled();
  });

  it('handles delete flow with confirmation dialog', () => {
    const handleDelete = jest.fn();

    render(
      <WidgetContainer title="Test Widget" onDelete={handleDelete}>
        Content
      </WidgetContainer>,
    );

    // Initial click opens dialog, does not trigger delete
    fireEvent.click(screen.getByLabelText(/Delete Test Widget/i));
    expect(screen.getByText('Delete Widget')).toBeInTheDocument();
    expect(handleDelete).not.toHaveBeenCalled();

    // Confirm deletion
    const dialogDeleteBtns = screen.getAllByRole('button', { name: /Delete/i });
    // One button might be the icon, the other is the text button in dialog. Let's find the text button.
    const confirmBtn = dialogDeleteBtns.find(
      (btn) => btn.textContent === 'Delete',
    );
    expect(confirmBtn).toBeDefined();

    fireEvent.click(confirmBtn!);
    expect(handleDelete).toHaveBeenCalledTimes(1);
  });

  it('handles inline title editing', () => {
    const handleTitleChange = jest.fn();

    render(
      <WidgetContainer title="Initial Title" onTitleChange={handleTitleChange}>
        Content
      </WidgetContainer>,
    );

    // Click edit icon
    fireEvent.click(screen.getByLabelText(/Edit title Initial Title/i));

    // Change title
    const input = screen.getByRole('textbox');
    fireEvent.change(input, { target: { value: 'New Name' } });

    // Press Enter to submit
    fireEvent.keyDown(input, { key: 'Enter', code: 'Enter' });

    expect(handleTitleChange).toHaveBeenCalledWith('New Name');

    // Title changes back to normal text after submit
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
  });

  it('cancels inline title editing with Escape', () => {
    const handleTitleChange = jest.fn();

    render(
      <WidgetContainer title="Initial Title" onTitleChange={handleTitleChange}>
        Content
      </WidgetContainer>,
    );

    // Click edit icon
    fireEvent.click(screen.getByLabelText(/Edit title Initial Title/i));

    const input = screen.getByRole('textbox');
    fireEvent.change(input, { target: { value: 'New Name' } });

    // Press Escape to cancel
    fireEvent.keyDown(input, { key: 'Escape', code: 'Escape' });

    expect(handleTitleChange).not.toHaveBeenCalled();
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
    expect(screen.getByText('Initial Title')).toBeInTheDocument();
  });
});
