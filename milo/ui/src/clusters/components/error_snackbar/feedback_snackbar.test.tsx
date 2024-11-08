// Copyright 2022 The LUCI Authors.
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

import '@testing-library/jest-dom';

import { render, screen } from '@testing-library/react';

import { Snack, SnackbarContext } from '@/clusters/context/snackbar_context';
import { identityFunction } from '@/clusters/testing_tools/functions';

import FeedbackSnackbar from './feedback_snackbar';

describe('Test ErrorSnackbar component', () => {
  it('given an error text, should display in a snackbar', async () => {
    const snack: Snack = {
      open: true,
      message: 'Failed to load issue',
      severity: 'error',
    };
    render(
      <SnackbarContext.Provider
        value={{
          snack: snack,
          setSnack: identityFunction,
        }}
      >
        <FeedbackSnackbar />
      </SnackbarContext.Provider>,
    );

    await screen.findByTestId('snackbar');

    expect(screen.getByText('Failed to load issue')).toBeInTheDocument();
  });
});
