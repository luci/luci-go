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

import { cleanup, render, screen } from '@testing-library/react';
import fetchMock from 'fetch-mock-jest';

import { ANSIText } from './ansi_text';

describe('<ANSIText />', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    cleanup();
    fetchMock.mockClear();
    fetchMock.reset();
    jest.useRealTimers();
  });

  it('should escape HTML', async () => {
    const { container } = render(
      <ANSIText
        content={`magic-string:\u001b[36m<div data-testid="should-be-escaped">\u001b[39m\n\u001b[36m</div>\u001b[39m`}
      />,
    );

    // The ANSI sequence following the magic string should be converted to an
    // HTML tag (beginning with a `<`).
    expect(container.innerHTML).toContain('magic-string:<span');

    // The HTML in the content should've been escaped.
    expect(screen.queryByTestId('should-be-escaped')).not.toBeInTheDocument();
  });
});
