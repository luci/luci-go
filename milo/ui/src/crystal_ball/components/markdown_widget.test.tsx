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

import { PerfWidget } from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import { MarkdownWidget } from './markdown_widget';

function createMockWidget(overrides: Partial<PerfWidget> = {}): PerfWidget {
  return {
    id: 'widget-default-id',
    displayName: 'Default Name',
    ...overrides,
  };
}

describe('MarkdownWidget', () => {
  const mockWidget = createMockWidget({
    id: 'widget-test',
    markdown: { content: 'Current status' },
  });

  it('renders initial markdown', () => {
    render(<MarkdownWidget widget={mockWidget} onUpdate={jest.fn()} />);
    expect(screen.getByText('Current status')).toBeInTheDocument();
  });

  it('switches to edit mode on icon click', () => {
    render(<MarkdownWidget widget={mockWidget} onUpdate={jest.fn()} />);
    fireEvent.click(screen.getByLabelText('Edit'));
    expect(screen.getByRole('textbox')).toHaveValue('Current status');
  });

  it('calls onUpdate and exits edit mode on Apply', () => {
    const handleUpdate = jest.fn();
    render(<MarkdownWidget widget={mockWidget} onUpdate={handleUpdate} />);

    fireEvent.click(screen.getByLabelText('Edit'));

    const textarea = screen.getByRole('textbox');
    fireEvent.change(textarea, { target: { value: 'Updated' } });

    fireEvent.click(screen.getByText('Apply'));

    expect(handleUpdate).toHaveBeenCalledWith({
      ...mockWidget,
      markdown: { content: 'Updated' },
    });

    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();

    // Test that the view correctly updates if passed the new widget prop
    render(
      <MarkdownWidget
        widget={{ ...mockWidget, markdown: { content: 'Updated' } }}
        onUpdate={handleUpdate}
      />,
    );
    expect(screen.getByText('Updated')).toBeInTheDocument();
  });

  it('cancels edits and exits edit mode on Cancel', () => {
    const handleUpdate = jest.fn();
    render(<MarkdownWidget widget={mockWidget} onUpdate={handleUpdate} />);

    fireEvent.click(screen.getByLabelText('Edit'));

    const textarea = screen.getByRole('textbox');
    fireEvent.change(textarea, { target: { value: 'Updated' } });

    fireEvent.click(screen.getByText('Cancel'));

    expect(handleUpdate).not.toHaveBeenCalled();
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
    expect(screen.getByText('Current status')).toBeInTheDocument();
  });

  it('renders hint when markdown is empty', () => {
    render(
      <MarkdownWidget
        widget={createMockWidget({
          id: 'widget-test',
          markdown: { content: '' },
        })}
        onUpdate={jest.fn()}
      />,
    );
    expect(screen.getByText('No content. Click to edit.')).toBeInTheDocument();
  });

  it('renders hint when markdown is whitespace only', () => {
    render(
      <MarkdownWidget
        widget={createMockWidget({
          id: 'widget-test',
          markdown: { content: '   ' },
        })}
        onUpdate={jest.fn()}
      />,
    );
    expect(screen.getByText('No content. Click to edit.')).toBeInTheDocument();
  });
});
