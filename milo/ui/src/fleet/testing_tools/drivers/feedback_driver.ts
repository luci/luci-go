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

import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

type UserEventInstance = ReturnType<typeof userEvent.setup>;

/**
 * Shared test driver for user feedback elements (MUI Alerts, Snackbars/toasts,
 * and confirmation dialogs) across Fleet Console pages.
 */
export const fleetFeedback = {
  /**
   * Asserts that an error alert with the given title and optional body message
   * is visible on screen.
   */
  async expectErrorAlert(title: string | RegExp, message?: string | RegExp) {
    expect(await screen.findByText(title)).toBeInTheDocument();
    if (message !== undefined) {
      expect(await screen.findByText(message)).toBeInTheDocument();
    }
  },

  /**
   * Asserts that an alert banner / snackbar toast containing `message` is
   * displayed.
   */
  async expectSnackbarMessage(message: string | RegExp) {
    const alert = await screen.findByRole('alert');
    expect(within(alert).getByText(message)).toBeInTheDocument();
  },

  /**
   * Confirms an open modal dialog by clicking its confirmation action button.
   */
  async confirmDialog(
    user: UserEventInstance,
    confirmButtonName: string | RegExp = /confirm/i,
  ) {
    const dialog = await screen.findByRole('dialog');
    const confirmButton = within(dialog).getByRole('button', {
      name: confirmButtonName,
    });
    await user.click(confirmButton);
  },
};
