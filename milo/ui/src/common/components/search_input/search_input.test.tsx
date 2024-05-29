// Copyright 2024 The LUCI Authors.
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
import { act } from 'react';

import { SearchInput } from './search_input';

describe('<SearchInput />', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
    cleanup();
  });

  it('can delay update callback', async () => {
    const onValueChangeSpy = jest.fn((_newValue: string) => {});
    render(
      <SearchInput
        value="val"
        onValueChange={onValueChangeSpy}
        initDelayMs={1000}
      />,
    );

    const searchInputEle = screen.getByTestId('search-input');
    expect(searchInputEle).toHaveValue('val');

    // Input search query.
    fireEvent.change(searchInputEle, { target: { value: 'new-val' } });
    expect(searchInputEle).toHaveValue('new-val');
    expect(onValueChangeSpy).not.toHaveBeenCalled();

    // Waiting for the search query to commit.
    await act(() => jest.advanceTimersByTimeAsync(500));
    expect(onValueChangeSpy).not.toHaveBeenCalled();

    // The search query is committed.
    await act(() => jest.advanceTimersByTimeAsync(1000));
    expect(searchInputEle).toHaveValue('new-val');
    expect(onValueChangeSpy).toHaveBeenCalledTimes(1);
    expect(onValueChangeSpy).toHaveBeenNthCalledWith(1, 'new-val');

    // No further updates.
    await act(() => jest.runAllTimersAsync());
    expect(onValueChangeSpy).toHaveBeenCalledTimes(1);
  });

  it('can cancel update callback when value is updated during pending period', async () => {
    const onValueChangeSpy = jest.fn((_newValue: string) => {});
    render(
      <SearchInput
        value="val"
        onValueChange={onValueChangeSpy}
        initDelayMs={1000}
      />,
    );

    const searchInputEle = screen.getByTestId('search-input');
    expect(searchInputEle).toHaveValue('val');

    // Input search query.
    fireEvent.change(searchInputEle, { target: { value: 'new-val' } });
    expect(searchInputEle).toHaveValue('new-val');
    expect(onValueChangeSpy).not.toHaveBeenCalled();

    // Waiting for the search query to commit.
    await act(() => jest.advanceTimersByTimeAsync(500));
    expect(onValueChangeSpy).not.toHaveBeenCalled();

    // Search query is updated again while pending.
    fireEvent.change(searchInputEle, { target: { value: 'newer-val' } });
    expect(searchInputEle).toHaveValue('newer-val');
    expect(onValueChangeSpy).not.toHaveBeenCalled();

    // Waiting for the new search query to commit.
    await act(() => jest.advanceTimersByTimeAsync(500));
    expect(onValueChangeSpy).not.toHaveBeenCalled();

    // The search query is committed.
    await act(() => jest.advanceTimersByTimeAsync(1000));
    expect(searchInputEle).toHaveValue('newer-val');
    expect(onValueChangeSpy).toHaveBeenCalledTimes(1);
    expect(onValueChangeSpy).toHaveBeenNthCalledWith(1, 'newer-val');

    // No further updates.
    await act(() => jest.runAllTimersAsync());
    expect(onValueChangeSpy).toHaveBeenCalledTimes(1);
  });

  it('can override pending value when parent set an update', async () => {
    const onValueChangeSpy = jest.fn((_newValue: string) => {});
    const { rerender } = render(
      <SearchInput
        value="val"
        onValueChange={onValueChangeSpy}
        initDelayMs={1000}
      />,
    );

    const searchInputEle = screen.getByTestId('search-input');
    expect(searchInputEle).toHaveValue('val');

    // Input search query.
    fireEvent.change(searchInputEle, { target: { value: 'new-val' } });
    expect(searchInputEle).toHaveValue('new-val');
    expect(onValueChangeSpy).not.toHaveBeenCalled();

    // Waiting for the search query to commit.
    await act(() => jest.advanceTimersByTimeAsync(500));
    expect(onValueChangeSpy).not.toHaveBeenCalled();

    // Parent sets a new value while pending.
    rerender(
      <SearchInput
        value="new-parent-val"
        onValueChange={onValueChangeSpy}
        initDelayMs={1000}
      />,
    );
    expect(searchInputEle).toHaveValue('new-parent-val');
    expect(onValueChangeSpy).not.toHaveBeenCalled();

    // No further updates.
    await act(() => jest.runAllTimersAsync());
    expect(onValueChangeSpy).not.toHaveBeenCalled();
  });

  it('do not override pending value when parent set an update due to onValueChange', async () => {
    const onValueChangeSpy = jest.fn((_newValue: string) => {});
    const { rerender } = render(
      <SearchInput
        value="val"
        onValueChange={onValueChangeSpy}
        initDelayMs={1000}
      />,
    );

    const searchInputEle = screen.getByTestId('search-input');
    expect(searchInputEle).toHaveValue('val');

    // Input search query.
    fireEvent.change(searchInputEle, { target: { value: 'new-val' } });
    expect(searchInputEle).toHaveValue('new-val');
    expect(onValueChangeSpy).not.toHaveBeenCalled();

    // Waiting for the search query to commit.
    await act(() => jest.advanceTimersByTimeAsync(1000));
    expect(onValueChangeSpy).toHaveBeenCalledTimes(1);

    // Edit the value again before parent's state update got applied.
    fireEvent.change(searchInputEle, { target: { value: 'new-val-2' } });

    // Parent updates the value to match the new value set by `onValueChange`
    rerender(
      <SearchInput
        value="new-val"
        onValueChange={onValueChangeSpy}
        initDelayMs={1000}
      />,
    );
    expect(searchInputEle).toHaveValue('new-val-2');
    expect(onValueChangeSpy).toHaveBeenCalledTimes(1);

    // After 1s, the 2nd edit is committed.
    await act(() => jest.advanceTimersByTimeAsync(1000));
    expect(searchInputEle).toHaveValue('new-val-2');
    expect(onValueChangeSpy).toHaveBeenCalledTimes(2);
    expect(onValueChangeSpy).toHaveBeenNthCalledWith(2, 'new-val-2');
  });

  it('can focus with shortcut', async () => {
    const onValueChangeSpy = jest.fn((_newValue: string) => {});
    render(
      <SearchInput
        value=""
        focusShortcut="/"
        onValueChange={onValueChangeSpy}
        initDelayMs={1000}
      />,
    );
    const searchInputEle = screen.getByTestId('search-input');
    expect(searchInputEle).not.toHaveFocus();

    await act(() => jest.runAllTimersAsync());
    fireEvent.keyDown(window, {
      key: '/',
      code: 'Slash',
      charCode: '/'.charCodeAt(0),
    });
    await act(() => jest.runAllTimersAsync());
    await act(() => jest.runAllTimersAsync());

    expect(searchInputEle).toHaveFocus();
    expect(onValueChangeSpy).not.toHaveBeenCalled();
  });
});
