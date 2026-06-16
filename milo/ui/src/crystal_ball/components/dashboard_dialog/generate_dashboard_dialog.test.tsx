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

import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import { GenerateDashboardDialog } from '@/crystal_ball/components/dashboard_dialog/generate_dashboard_dialog';
import { useSuggestMeasurementFilterValues } from '@/crystal_ball/hooks';
import { createMockQueryResult } from '@/crystal_ball/tests';

jest.mock('@/crystal_ball/hooks', () => ({
  useSuggestMeasurementFilterValues: jest.fn(),
}));

describe('<GenerateDashboardDialog />', () => {
  const mockOnClose = jest.fn();
  const mockOnSubmit = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();
    jest.mocked(useSuggestMeasurementFilterValues).mockReturnValue(
      createMockQueryResult({
        suggestions: [
          { value: 'metric_key_1', count: '10' },
          { value: 'metric_key_2', count: '5' },
        ],
        values: ['metric_key_1', 'metric_key_2'],
      }),
    );
  });

  it('renders correctly when open', () => {
    render(
      <GenerateDashboardDialog
        onClose={mockOnClose}
        onSubmit={mockOnSubmit}
        open={true}
      />,
    );

    expect(screen.getByText('Generate Dashboard')).toBeInTheDocument();
    expect(
      screen.getByLabelText('What kind of dashboard do you want?'),
    ).toHaveValue('');
    expect(screen.getByLabelText('Associated Metric Keys')).toBeInTheDocument();
    expect(screen.getByLabelText('Primary AnTS Invocation ID')).toHaveValue('');
    expect(screen.getByLabelText('Comparison AnTS Invocation ID')).toHaveValue(
      '',
    );
    expect(screen.getByRole('button', { name: 'Generate' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Cancel' })).not.toBeDisabled();
  });

  it('submits the form with prompt only', async () => {
    render(
      <GenerateDashboardDialog
        onClose={mockOnClose}
        onSubmit={mockOnSubmit}
        open={true}
      />,
    );

    const promptInput = screen.getByLabelText(
      'What kind of dashboard do you want?',
    );
    fireEvent.change(promptInput, {
      target: { value: 'A beautiful new dashboard' },
    });

    const generateButton = screen.getByRole('button', { name: 'Generate' });
    expect(generateButton).not.toBeDisabled();
    fireEvent.click(generateButton);

    await waitFor(() => {
      expect(mockOnSubmit).toHaveBeenCalledWith({
        antsInvocationId: undefined,
        comparisonAntsInvocationId: undefined,
        metricKeys: [],
        prompt: 'A beautiful new dashboard',
      });
    });
  });

  it('submits the form with prompt, invocation IDs and metric keys', async () => {
    render(
      <GenerateDashboardDialog
        onClose={mockOnClose}
        onSubmit={mockOnSubmit}
        open={true}
      />,
    );

    const promptInput = screen.getByLabelText(
      'What kind of dashboard do you want?',
    );
    fireEvent.change(promptInput, {
      target: { value: 'A beautiful new dashboard' },
    });

    const primaryIdInput = screen.getByLabelText('Primary AnTS Invocation ID');
    fireEvent.change(primaryIdInput, { target: { value: 'I12345' } });

    const comparisonIdInput = screen.getByLabelText(
      'Comparison AnTS Invocation ID',
    );
    fireEvent.change(comparisonIdInput, { target: { value: 'I67890' } });

    const generateButton = screen.getByRole('button', { name: 'Generate' });
    expect(generateButton).not.toBeDisabled();
    fireEvent.click(generateButton);

    await waitFor(() => {
      expect(mockOnSubmit).toHaveBeenCalledWith({
        antsInvocationId: 'I12345',
        comparisonAntsInvocationId: 'I67890',
        metricKeys: [],
        prompt: 'A beautiful new dashboard',
      });
    });
  });

  it('renders pending state correctly', () => {
    render(
      <GenerateDashboardDialog
        isPending={true}
        onClose={mockOnClose}
        onSubmit={mockOnSubmit}
        open={true}
      />,
    );

    expect(
      screen.getByRole('button', { name: 'Generating...' }),
    ).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled();
  });

  it('renders error message correctly', () => {
    render(
      <GenerateDashboardDialog
        errorMsg="Something went wrong"
        onClose={mockOnClose}
        onSubmit={mockOnSubmit}
        open={true}
      />,
    );

    expect(screen.getByText('Something went wrong')).toBeInTheDocument();
  });

  it('renders autocomplete dropdown with a higher z-index than the drawer', async () => {
    render(
      <GenerateDashboardDialog
        onClose={mockOnClose}
        onSubmit={mockOnSubmit}
        open={true}
      />,
    );

    const drawer = screen.getByRole('dialog');
    const drawerStyle = window.getComputedStyle(drawer);
    const drawerZIndex = parseInt(drawerStyle.zIndex, 10);

    const autocompleteInput = screen.getByLabelText('Metric Keys');
    fireEvent.focus(autocompleteInput);
    fireEvent.change(autocompleteInput, { target: { value: 'metric' } });

    await waitFor(() => {
      expect(screen.getByText('metric_key_1')).toBeInTheDocument();
    });

    const optionElement = screen.getByText('metric_key_1');
    const popper = optionElement.closest('.MuiAutocomplete-popper');
    expect(popper).not.toBeNull();

    const popperStyle = window.getComputedStyle(popper!);
    const popperZIndex = parseInt(popperStyle.zIndex, 10);
    expect(popperZIndex).toBeGreaterThan(drawerZIndex);
  });
});
