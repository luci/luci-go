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
 * Shared test driver for Fleet Console filter bars and search inputs.
 */
export const fleetFilterBar = {
  /**
   * Types an AIP-160 filter expression into the filter search input (scoped to
   * `container` if provided) and commits it with `{Enter}`.
   */
  async applyFilter(
    user: UserEventInstance,
    filterText: string,
    options?: {
      placeholder?: string | RegExp;
      container?: HTMLElement;
    },
  ) {
    const scope = options?.container ? within(options.container) : screen;
    const input = options?.placeholder
      ? scope.getByPlaceholderText(options.placeholder)
      : scope.getByRole('textbox');
    await user.click(input);
    await user.paste(filterText);
    await user.keyboard('{Enter}');
  },

  /**
   * Asserts that a filter chip containing `label` is rendered.
   */
  async expectChipVisible(label: string | RegExp) {
    expect(await screen.findByText(label)).toBeInTheDocument();
  },

  /**
   * Clicks the clear-filters control when present.
   */
  async clearAll(user: UserEventInstance) {
    const clearButton = await screen.findByRole('button', {
      name: /clear/i,
    });
    await user.click(clearButton);
  },
};
