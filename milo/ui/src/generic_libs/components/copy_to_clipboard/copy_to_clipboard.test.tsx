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
import copy from 'copy-to-clipboard';
import { act } from 'react';

import { CopyToClipboard } from './copy_to_clipboard';

jest.mock('copy-to-clipboard');

describe('<CopyToClipboard />', () => {
  let copyMock: jest.MockedFunction<typeof copy>;

  beforeEach(() => {
    jest.useFakeTimers();
    copyMock = jest.mocked(copy);
  });

  afterEach(() => {
    cleanup();
    jest.useRealTimers();
  });

  it('should copy text when clicked', () => {
    render(<CopyToClipboard textToCopy="teststring" />);

    fireEvent.click(screen.getByTestId('ContentCopyIcon'));
    expect(copyMock).toHaveBeenCalledWith('teststring');
  });

  it('should support copying text generated from function', () => {
    const genText = jest.fn(() => 'teststring');
    render(<CopyToClipboard textToCopy={genText} />);
    expect(genText).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTestId('ContentCopyIcon'));
    expect(genText).toHaveBeenCalledTimes(1);
    expect(copyMock).toHaveBeenCalledWith('teststring');
  });

  it('should display done icon for a short period when clicked', async () => {
    render(<CopyToClipboard textToCopy="teststring" />);
    expect(screen.getByTestId('ContentCopyIcon')).toBeInTheDocument();
    expect(screen.queryByTestId('DoneIcon')).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId('ContentCopyIcon'));
    expect(screen.queryByTestId('ContentCopyIcon')).not.toBeInTheDocument();
    expect(screen.getByTestId('DoneIcon')).toBeInTheDocument();

    await act(() => jest.advanceTimersByTimeAsync(500));
    expect(screen.queryByTestId('ContentCopyIcon')).not.toBeInTheDocument();
    expect(screen.getByTestId('DoneIcon')).toBeInTheDocument();

    await act(() => jest.advanceTimersByTimeAsync(1000));
    expect(screen.getByTestId('ContentCopyIcon')).toBeInTheDocument();
    expect(screen.queryByTestId('DoneIcon')).not.toBeInTheDocument();
  });
});
