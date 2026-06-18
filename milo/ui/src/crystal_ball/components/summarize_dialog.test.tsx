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

import {
  render,
  screen,
  fireEvent,
  waitFor,
  act,
} from '@testing-library/react';

import { SummarizeDialog } from './summarize_dialog';

describe('<SummarizeDialog />', () => {
  const mockOnClose = jest.fn();
  const mockOnSummarize = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('renders correctly when open', () => {
    render(
      <SummarizeDialog
        open={true}
        onClose={mockOnClose}
        title="Summarize Test"
        onSummarize={mockOnSummarize}
      />,
    );

    expect(screen.getByText('Summarize Test')).toBeInTheDocument();
    expect(
      screen.getByLabelText(/AI Guidance Instructions/i),
    ).toBeInTheDocument();
    expect(screen.getByText('Cancel')).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'Summarize' }),
    ).toBeInTheDocument();
  });

  it('does not render when closed', () => {
    render(
      <SummarizeDialog
        open={false}
        onClose={mockOnClose}
        title="Summarize Test"
        onSummarize={mockOnSummarize}
      />,
    );

    expect(screen.queryByText('Summarize Test')).not.toBeInTheDocument();
  });

  it('calls onSummarize and shows loader, then displays result', async () => {
    mockOnSummarize.mockResolvedValue(
      '## Summary Result\nThis is a cool summary!',
    );
    render(
      <SummarizeDialog
        open={true}
        onClose={mockOnClose}
        title="Summarize Test"
        onSummarize={mockOnSummarize}
      />,
    );

    const input = screen.getByLabelText(/AI Guidance Instructions/i);
    fireEvent.change(input, { target: { value: 'Focus on recent errors' } });

    const summarizeBtn = screen.getByRole('button', { name: 'Summarize' });
    fireEvent.click(summarizeBtn);

    // Should show loader
    expect(screen.getByText('Generating summary...')).toBeInTheDocument();
    expect(mockOnSummarize).toHaveBeenCalledWith('Focus on recent errors');

    // Wait for resolution and summary rendering
    await waitFor(() => {
      expect(screen.getByText('AI-Generated Summary:')).toBeInTheDocument();
    });
    expect(
      screen.getByText('Summary completed successfully.'),
    ).toBeInTheDocument();
    expect(screen.getByText('This is a cool summary!')).toBeInTheDocument();
    expect(screen.queryByText('Generating summary...')).not.toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'Copy to Clipboard' }),
    ).toBeInTheDocument();
  });

  it('shows error if onSummarize fails', async () => {
    mockOnSummarize.mockRejectedValue(
      new Error('AI failed to generate summary'),
    );
    render(
      <SummarizeDialog
        open={true}
        onClose={mockOnClose}
        title="Summarize Test"
        onSummarize={mockOnSummarize}
      />,
    );

    const summarizeBtn = screen.getByRole('button', { name: 'Summarize' });
    fireEvent.click(summarizeBtn);

    await waitFor(() => {
      expect(
        screen.getByText('AI failed to generate summary'),
      ).toBeInTheDocument();
    });
  });

  it('resets dialog state on click of Ask again', async () => {
    mockOnSummarize.mockResolvedValue('Summary content');
    render(
      <SummarizeDialog
        open={true}
        onClose={mockOnClose}
        title="Summarize Test"
        onSummarize={mockOnSummarize}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Summarize' }));

    await waitFor(() => {
      expect(screen.getByText('AI-Generated Summary:')).toBeInTheDocument();
    });

    const askAgainBtn = screen.getByRole('button', { name: 'Ask again' });
    fireEvent.click(askAgainBtn);

    expect(screen.queryByText('AI-Generated Summary:')).not.toBeInTheDocument();
    expect(
      screen.getByLabelText(/AI Guidance Instructions/i),
    ).toBeInTheDocument();
  });

  it('preserves prompt and summary state when closed and reopened', async () => {
    mockOnSummarize.mockResolvedValue('Summary content');
    const { rerender } = render(
      <SummarizeDialog
        open={true}
        onClose={mockOnClose}
        title="Summarize Test"
        onSummarize={mockOnSummarize}
      />,
    );

    const input = screen.getByLabelText(/AI Guidance Instructions/i);
    fireEvent.change(input, { target: { value: 'custom instruction' } });
    fireEvent.click(screen.getByRole('button', { name: 'Summarize' }));

    await waitFor(() => {
      expect(screen.getByText('Summary content')).toBeInTheDocument();
    });

    // Close it
    rerender(
      <SummarizeDialog
        open={false}
        onClose={mockOnClose}
        title="Summarize Test"
        onSummarize={mockOnSummarize}
      />,
    );

    // Reopen it
    rerender(
      <SummarizeDialog
        open={true}
        onClose={mockOnClose}
        title="Summarize Test"
        onSummarize={mockOnSummarize}
      />,
    );

    // Prompt and summary should still be present
    expect(screen.getByText('Summary content')).toBeInTheDocument();

    // Click Ask again to check if prompt state was preserved
    const askAgainBtn = screen.getByRole('button', { name: 'Ask again' });
    fireEvent.click(askAgainBtn);

    expect(screen.getByDisplayValue('custom instruction')).toBeInTheDocument();
  });

  it('allows closing the drawer while summarization is processing', async () => {
    let resolveSummarize: (value: string) => void = () => {};
    mockOnSummarize.mockImplementation(() => {
      return new Promise<string>((resolve) => {
        resolveSummarize = resolve;
      });
    });

    render(
      <SummarizeDialog
        open={true}
        onClose={mockOnClose}
        title="Summarize Test"
        onSummarize={mockOnSummarize}
      />,
    );

    const summarizeBtn = screen.getByRole('button', { name: 'Summarize' });
    fireEvent.click(summarizeBtn);

    // Dialog is loading
    expect(screen.getByText('Generating summary...')).toBeInTheDocument();

    // Close button should NOT be disabled, and clicking it should call onClose
    const closeBtn = screen.getByRole('button', { name: 'Cancel' });
    expect(closeBtn).not.toBeDisabled();
    fireEvent.click(closeBtn);

    expect(mockOnClose).toHaveBeenCalled();

    // Clean up promise
    await act(async () => {
      resolveSummarize('done');
    });
  });

  it('copies summary content to clipboard on click of Copy to Clipboard', async () => {
    const mockClipboardWriteText = jest.fn();
    Object.assign(navigator, {
      clipboard: {
        writeText: mockClipboardWriteText,
      },
    });

    mockOnSummarize.mockResolvedValue('Summary content');
    render(
      <SummarizeDialog
        open={true}
        onClose={mockOnClose}
        title="Summarize Test"
        onSummarize={mockOnSummarize}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Summarize' }));

    await waitFor(() => {
      expect(screen.getByText('Summary content')).toBeInTheDocument();
    });

    const copyBtn = screen.getByRole('button', { name: 'Copy to Clipboard' });
    fireEvent.click(copyBtn);

    expect(mockClipboardWriteText).toHaveBeenCalledWith('Summary content');
    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: 'Copied!' }),
      ).toBeInTheDocument();
    });
  });
});
