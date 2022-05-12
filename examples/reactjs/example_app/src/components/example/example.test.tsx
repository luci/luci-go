// Copyright 2022 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.


/**
 * This adds testing-library matchers to jest.
 */
import '@testing-library/jest-dom';
import 'node-fetch';

/**
 * This import must be declared before any use of `fetch`
 * in your code if you want to mock the `fetch` calls.
 *
 * If any call is made to `fetch` before this gets to run (for example,
 * if you run it in a constructor of a class), your mocks will fail.
 */
import fetchMock from 'fetch-mock-jest';

import {
  render,
  screen,
} from '@testing-library/react';

import Example from './example';

describe('<Example />', () => {
  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  it('Wait for component to load', async () => {
    render(
        <Example exampleProp='test'/>,
    );

    await screen.findByRole('article');

    expect(screen.getByText('test')).toBeInTheDocument();
  });
});
