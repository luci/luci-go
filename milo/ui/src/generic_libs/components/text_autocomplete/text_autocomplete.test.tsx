// Copyright 2025 The LUCI Authors.
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

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { act } from 'react';

import { TextAutocomplete } from './text_autocomplete';
import { OptionDef } from './types';

describe('TextAutocomplete', () => {
  const options: OptionDef<string>[] = [
    { id: 'option-1', value: 'Option 1' },
    { id: 'option-2', value: 'Option 2', unselectable: true },
    { id: 'option-3', value: 'Option 3' },
  ];

  const renderOption = (option: OptionDef<string>) => <td>{option.value}</td>;

  const applyOption = (
    _value: string,
    _cursorPos: number,
    option: OptionDef<string>,
  ) => [option.value, option.value.length] as const;

  const onValueCommit = jest.fn();
  const onRequestOptionsUpdate = jest.fn();

  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    cleanup();
    jest.useRealTimers();
    onValueCommit.mockClear();
    onRequestOptionsUpdate.mockClear();
  });

  it('renders options on input change', async () => {
    render(
      <TextAutocomplete
        value=""
        onValueCommit={onValueCommit}
        options={options}
        onRequestOptionsUpdate={onRequestOptionsUpdate}
        renderOption={renderOption}
        applyOption={applyOption}
      />,
    );

    fireEvent.keyDown(screen.getByRole('textbox'), {
      code: 'ArrowDown',
    });

    expect(screen.getByText('Option 1')).toBeVisible();
    // Even though unselectable, it should still render.
    expect(screen.getByText('Option 2')).toBeVisible();
    expect(screen.getByText('Option 3')).toBeVisible();
  });

  it('selects option with arrow keys and enter', async () => {
    render(
      <TextAutocomplete
        value="value"
        onValueCommit={onValueCommit}
        options={options}
        onRequestOptionsUpdate={onRequestOptionsUpdate}
        renderOption={renderOption}
        applyOption={applyOption}
        slotProps={{
          textField: {
            slotProps: {
              input: {
                inputProps: {
                  'data-testid': 'autocomplete-input',
                },
              },
            },
          },
        }}
      />,
    );

    // Open suggestions and select the first option.
    fireEvent.keyDown(screen.getByRole('textbox'), {
      code: 'ArrowDown',
    });
    expect(screen.getByText('Option 1').closest('tr')).toHaveClass('selected');
    expect(screen.getByText('Option 2').closest('tr')).not.toHaveClass(
      'selected',
    );
    expect(screen.getByText('Option 3').closest('tr')).not.toHaveClass(
      'selected',
    );

    // Select the next option.
    fireEvent.keyDown(screen.getByRole('textbox'), {
      code: 'ArrowDown',
    });
    expect(screen.getByText('Option 1').closest('tr')).not.toHaveClass(
      'selected',
    );
    expect(screen.getByText('Option 2').closest('tr')).not.toHaveClass(
      'selected',
    );
    expect(screen.getByText('Option 3').closest('tr')).toHaveClass('selected');

    // Apply the selected option.
    fireEvent.keyDown(screen.getByRole('textbox'), { code: 'Enter' });
    expect(screen.getByRole('textbox')).toHaveValue('Option 3');
    expect(onValueCommit).not.toHaveBeenCalled();

    // Commit the value.
    fireEvent.keyDown(screen.getByRole('textbox'), { code: 'Enter' });
    expect(onValueCommit).toHaveBeenCalledWith('Option 3');
  });

  it('can clear value', async () => {
    render(
      <TextAutocomplete
        value="old-value"
        onValueCommit={onValueCommit}
        options={[]}
        onRequestOptionsUpdate={onRequestOptionsUpdate}
        renderOption={renderOption}
        applyOption={applyOption}
      />,
    );

    fireEvent.click(screen.getByTestId('CloseIcon'));
    expect(onValueCommit).toHaveBeenCalledWith('');
  });

  // We want to prevent the bug where the click on the option is not registered
  // due to the same click causing the option dropdown to be dismissed before
  // option element has a chance to react to the click.
  //
  // We cannot test "whether clicking on an option will trigger the onclick
  // event of that option" directly. This is because Jest is unable to simulate
  // clicking on an element without triggering the onclick event handler on that
  // element. In the real browser, the click is registered on a coordinate on
  // on the screen. The element may not receive the onclick event. In Jest, the
  // click event is dispatched to the selected target directly. The onclick
  // handler is always triggered.
  //
  // As a compromise, we check whether clicking on an unselectable option will
  // cause the dropdown to the dismissed.
  it('clicking on an unselectable option does not dismiss options', async () => {
    render(
      <TextAutocomplete
        value=""
        onValueCommit={onValueCommit}
        options={options}
        onRequestOptionsUpdate={onRequestOptionsUpdate}
        renderOption={renderOption}
        applyOption={applyOption}
      />,
    );

    // Open the options first.
    fireEvent.keyDown(screen.getByRole('textbox'), {
      code: 'ArrowDown',
    });
    expect(screen.getByText('Option 2')).toBeVisible();

    // Clicking on an unselectable option.
    const user = userEvent.setup({ advanceTimers: jest.advanceTimersByTime });
    await user.click(screen.getByText('Option 2'));

    // Wait a short period to let UI updates (if any) apply.
    await act(() => jest.advanceTimersByTimeAsync(100));
    expect(screen.getByText('Option 2')).toBeVisible();
  });
});
