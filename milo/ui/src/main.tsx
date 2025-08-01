// Copyright 2020 The LUCI Authors.
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

import 'virtual:override-milo-host';
import '@/common/api/error_reporting/register_handlers';
import '@/proto_utils/duration_patch';

import { Settings } from 'luxon';
import { configure } from 'mobx';
import { createRoot } from 'react-dom/client';

import { App } from '@/App';
import { initDefaultTrustedTypesPolicy } from '@/common/tools/sanitize_html';
import { IsDevBuildProvider } from '@/generic_libs/hooks/is_dev_build';
import { assertNonNullable } from '@/generic_libs/tools/utils';
import { initUiSW } from '@/sw/init_sw';

/**
 * Whether the UI service worker should be enabled.
 */
declare const ENABLE_UI_SW: boolean;

if (
  navigator.serviceWorker &&
  ENABLE_UI_SW &&
  // Allow cypress to disable service worker via a global variable.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  !(window as any).DISABLE_UI_SW_FOR_CYPRESS
) {
  initUiSW({ isDevEnv: import.meta.env.DEV });
}

initDefaultTrustedTypesPolicy();

// TODO(crbug/1347294): encloses all state modifying actions in mobx actions
// then delete this.
configure({ enforceActions: 'never' });

const container = assertNonNullable(document.getElementById('app-root'));
const root = createRoot(container);
root.render(
  <IsDevBuildProvider value={import.meta.env.DEV}>
    <App />
  </IsDevBuildProvider>,
);

Settings.throwOnInvalid = true;
declare module 'luxon' {
  interface TSSettings {
    throwOnInvalid: true;
  }
}
