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

import { genericThen } from '@/fleet/testing_tools/integration/fleet_integration_test';

export const then = {
  ...genericThen,

  mainLandmarkIsVisible() {
    expect(screen.getByRole('main')).toBeInTheDocument();
  },

  async mainLandmarkIsRendered() {
    expect(await screen.findByRole('main')).toBeInTheDocument();
  },

  mainLandmarkIsNotVisible() {
    expect(screen.queryByRole('main')).not.toBeInTheDocument();
  },

  async siteCardIsVisible(name: string, id: string) {
    const card = await screen.findByTestId(`site-card-${id}`);
    expect(card).toHaveAttribute('aria-label', name);
    expect(within(card).getByRole('heading', { name })).toBeInTheDocument();
    expect(within(card).getByText(id)).toBeInTheDocument();
  },

  async siteCardOrderIs(expectedNames: string[]) {
    if (expectedNames.length > 0) {
      await screen.findByRole('article', { name: expectedNames[0] });
    }
    const articles = screen.getAllByRole('article');
    expect(articles.map((el) => el.getAttribute('aria-label'))).toEqual(
      expectedNames,
    );
  },

  async siteCardIdOrderIs(expectedIds: string[]) {
    if (expectedIds.length > 0) {
      await screen.findByTestId(`site-card-${expectedIds[0]}`);
    }
    const articles = screen.getAllByRole('article');
    expect(articles.map((el) => el.getAttribute('data-testid'))).toEqual(
      expectedIds.map((id) => `site-card-${id}`),
    );
  },

  async siteFormSubmitIsDisabled() {
    const dialog = await screen.findByRole('dialog');
    const submitBtn = within(dialog).getByRole('button', {
      name: /Add site/i,
    });
    expect(submitBtn).toBeDisabled();
  },
};
