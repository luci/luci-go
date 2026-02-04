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

import { EditableMarkdown } from './editable_markdown';

describe('EditableMarkdown', () => {
  it('renders initial markdown', () => {
    render(
      <EditableMarkdown initialMarkdown="Current status" onSave={jest.fn()} />,
    );

    expect(screen.getByText('Current status')).toBeInTheDocument();
  });

  it('switches to edit mode on icon click', () => {
    render(
      <EditableMarkdown initialMarkdown="Current status" onSave={jest.fn()} />,
    );

    fireEvent.click(screen.getByTitle('Edit'));

    expect(screen.getByRole('textbox')).toHaveValue('Current status');
  });

  it('calls onSave and exits edit mode on blur', () => {
    const handleSave = jest.fn();
    render(<EditableMarkdown initialMarkdown="Initial" onSave={handleSave} />);

    fireEvent.click(screen.getByTitle('Edit'));

    const textarea = screen.getByRole('textbox');
    fireEvent.change(textarea, { target: { value: 'Updated' } });

    fireEvent.blur(textarea);

    expect(handleSave).toHaveBeenCalledWith('Updated');

    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
    expect(screen.getByText('Updated')).toBeInTheDocument();
  });

  it('renders hint when markdown is empty', () => {
    render(<EditableMarkdown initialMarkdown="" onSave={jest.fn()} />);
    expect(screen.getByText('No content. Click to edit.')).toBeInTheDocument();
  });

  it('renders hint when markdown is whitespace only', () => {
    render(<EditableMarkdown initialMarkdown="   " onSave={jest.fn()} />);
    expect(screen.getByText('No content. Click to edit.')).toBeInTheDocument();
  });
});
