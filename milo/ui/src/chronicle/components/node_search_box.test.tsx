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

import { NodeSearchBox } from './node_search_box';

describe('NodeSearchBox', () => {
  const defaultProps = {
    searchQuery: '',
    matchedNodeIds: [],
    safeMatchIndex: 0,
    isCurrentMatchSelected: true,
    selectedNodeId: undefined,
    onSearchChange: jest.fn(),
    onClearSearch: jest.fn(),
    onNextMatch: jest.fn(),
    onPrevMatch: jest.fn(),
    onStepToMatch: jest.fn(),
  };

  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('renders enabled search input by default', () => {
    render(<NodeSearchBox {...defaultProps} />);

    const input = screen.getByPlaceholderText('Search nodes...');
    expect(input).toBeEnabled();
    expect(input).not.toBeDisabled();

    fireEvent.change(input, { target: { value: 'build' } });
    expect(defaultProps.onSearchChange).toHaveBeenCalledWith('build');
  });

  it('disables input when disabled prop is true', () => {
    render(<NodeSearchBox {...defaultProps} disabled={true} />);

    const input = screen.getByPlaceholderText('Search nodes...');
    expect(input).toBeDisabled();
    expect(input).toHaveStyle({ cursor: 'not-allowed' });
  });

  it('disables match buttons and hides clear button when disabled is true', () => {
    render(
      <NodeSearchBox
        {...defaultProps}
        disabled={true}
        searchQuery="build"
        matchedNodeIds={['node-1', 'node-2']}
        safeMatchIndex={0}
      />,
    );

    const input = screen.getByPlaceholderText('Search nodes...');
    expect(input).toBeDisabled();

    // Clear button should be hidden when disabled
    expect(
      screen.queryByRole('button', { name: 'Clear search' }),
    ).not.toBeInTheDocument();

    // Prev and Next buttons should be disabled
    const prevButton = screen.getByRole('button', { name: 'Previous match' });
    const nextButton = screen.getByRole('button', { name: 'Next match' });
    expect(prevButton).toBeDisabled();
    expect(nextButton).toBeDisabled();
  });

  it('enables match buttons and shows clear button when enabled with matches', () => {
    render(
      <NodeSearchBox
        {...defaultProps}
        disabled={false}
        searchQuery="build"
        matchedNodeIds={['node-1', 'node-2']}
        safeMatchIndex={0}
      />,
    );

    const input = screen.getByPlaceholderText('Search nodes...');
    expect(input).toBeEnabled();

    const clearButton = screen.getByRole('button', { name: 'Clear search' });
    expect(clearButton).toBeInTheDocument();
    fireEvent.click(clearButton);
    expect(defaultProps.onClearSearch).toHaveBeenCalledTimes(1);

    const prevButton = screen.getByRole('button', { name: 'Previous match' });
    const nextButton = screen.getByRole('button', { name: 'Next match' });
    expect(prevButton).toBeEnabled();
    expect(nextButton).toBeEnabled();
  });
});
