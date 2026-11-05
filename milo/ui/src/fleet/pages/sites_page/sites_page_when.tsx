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

import {
  genericWhen,
  getUser,
} from '@/fleet/testing_tools/integration/fleet_integration_test';

import { Component as SitesRouteComponent, SitesPage } from './sites_page';

export const when = {
  ...genericWhen,

  pageIsRendered() {
    return genericWhen.renderPage(<SitesPage />);
  },

  routeComponentIsRendered() {
    return genericWhen.renderPage(<SitesRouteComponent />);
  },

  async userSearchesSites(query: string) {
    const input = await screen.findByRole('textbox', {
      name: 'Filter physical sites',
    });
    const user = getUser();
    await user.clear(input);
    if (query) {
      await user.type(input, query);
    }
  },

  async userOpensAddSiteDialog() {
    await genericWhen.userClicksButton('Add site');
  },

  async userFillsSiteForm(fields: { id?: string; name?: string }) {
    const dialog = await screen.findByRole('dialog');
    const user = getUser();
    if (fields.id !== undefined) {
      const idInput = within(dialog).getByLabelText(/Site code \(ID\)/i);
      await user.clear(idInput);
      if (fields.id) {
        await user.type(idInput, fields.id);
      }
    }
    if (fields.name !== undefined) {
      const nameInput = within(dialog).getByLabelText(/Display name/i);
      await user.clear(nameInput);
      if (fields.name) {
        await user.type(nameInput, fields.name);
      }
    }
  },

  async userSubmitsSiteForm() {
    const dialog = await screen.findByRole('dialog');
    const submitBtn = within(dialog).getByRole('button', {
      name: /Add site|Save changes/i,
    });
    await getUser().click(submitBtn);
  },

  async userOpensEditSiteDialog(siteName: string) {
    await genericWhen.userClicksButton(`Edit ${siteName}`);
  },

  async userOpensDeleteSiteDialog(siteName: string) {
    await genericWhen.userClicksButton(`Delete ${siteName}`);
  },

  async userConfirmsDeleteSite() {
    const dialog = await screen.findByRole('dialog');
    const deleteBtn = within(dialog).getByRole('button', {
      name: 'Delete site',
    });
    await getUser().click(deleteBtn);
  },
};
