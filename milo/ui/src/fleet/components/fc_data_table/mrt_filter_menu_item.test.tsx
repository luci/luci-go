import { fireEvent, render, screen } from '@testing-library/react';

import { MRTFilterMenuItem } from './mrt_filter_menu_item';

// TODO: Update this mock to use the new unified filter bar components once OptionsMenuOld is fully removed.
jest.mock('@/fleet/components/filter_dropdown/options_menu_old', () => ({
  OptionsMenuOld: ({
    elements,
    flipOption,
  }: {
    elements: { el: { value: string; label: string } }[];
    flipOption: (val: string) => void;
  }) => (
    <div data-testid="mock-options-menu">
      {/* eslint-disable-next-line @typescript-eslint/no-explicit-any */}
      {elements.map((x: any) => (
        <button key={x.el.value} onClick={() => flipOption(x.el.value)}>
          {x.el.label}
        </button>
      ))}
    </div>
  ),
}));

describe('<MRTFilterMenuItem />', () => {
  const mockCloseMenu = jest.fn();
  const mockSetFilterValue = jest.fn();
  const mockGetFilterValue = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();
    mockGetFilterValue.mockReturnValue(undefined);
  });

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const createMockColumn = (options?: unknown[]): any => {
    return {
      id: 'test_column',
      columnDef: {
        header: 'Test Column',
        filterSelectOptions: options,
      },
      getFilterValue: mockGetFilterValue,
      setFilterValue: mockSetFilterValue,
    };
  };

  it('renders disabled when no filter options exist', () => {
    const column = createMockColumn(undefined);
    render(<MRTFilterMenuItem column={column} closeMenu={mockCloseMenu} />);

    const menuItem = screen.getByText('Filter').closest('li');
    expect(menuItem).toHaveAttribute('aria-disabled', 'true');
  });

  it('renders enabled when filter options exist', () => {
    const column = createMockColumn(['option1', 'option2']);
    render(<MRTFilterMenuItem column={column} closeMenu={mockCloseMenu} />);

    const menuItem = screen.getByText('Filter').closest('li');
    expect(menuItem).not.toHaveAttribute('aria-disabled', 'true');
  });

  it('opens dropdown and displays options on click', () => {
    const column = createMockColumn(['option1', 'option2']);
    render(<MRTFilterMenuItem column={column} closeMenu={mockCloseMenu} />);

    fireEvent.click(screen.getByText('Filter'));

    // The options dropdown should be portal-rendered and visible
    expect(screen.getByText('option1')).toBeVisible();
    expect(screen.getByText('option2')).toBeVisible();
  });

  it('applies selected filters and closes menu', () => {
    const column = createMockColumn(['option1', 'option2']);
    render(<MRTFilterMenuItem column={column} closeMenu={mockCloseMenu} />);

    fireEvent.click(screen.getByText('Filter'));

    const option1 = screen.getByText('option1');
    fireEvent.click(option1);

    const applyButton = screen.getByRole('button', { name: /apply/i });
    fireEvent.click(applyButton);

    expect(mockSetFilterValue).toHaveBeenCalledWith(['option1']);
    expect(mockCloseMenu).toHaveBeenCalled();
  });

  it('resets selected filters back to undefined if none selected', () => {
    mockGetFilterValue.mockReturnValue(['option1']);
    const column = createMockColumn(['option1', 'option2']);
    render(<MRTFilterMenuItem column={column} closeMenu={mockCloseMenu} />);

    fireEvent.click(screen.getByText('Filter'));

    // Deselect option1
    const option1 = screen.getByText('option1');
    fireEvent.click(option1);

    const applyButton = screen.getByRole('button', { name: /apply/i });
    fireEvent.click(applyButton);

    expect(mockSetFilterValue).toHaveBeenCalledWith(undefined);
    expect(mockCloseMenu).toHaveBeenCalled();
  });
});
