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

import { configure } from 'mobx';
import { createRoot } from 'react-dom/client';

import './stackdriver_errors';
import { App } from './App';
import { initDefaultTrustedTypesPolicy } from './libs/sanitize_html';

/**
 * Whether the UI service worker should be enabled.
 */
declare const ENABLE_UI_SW: boolean;
/**
 * Whether GA tracking should be enabled.
 */
declare const ENABLE_GA: boolean;
window.ENABLE_GA = ENABLE_GA;

initDefaultTrustedTypesPolicy();

// TODO(crbug/1347294): encloses all state modifying actions in mobx actions
// then delete this.
configure({ enforceActions: 'never' });

// Reload the page after a new version is activated to avoid different versions
// of the code talking to each other.
navigator.serviceWorker?.addEventListener('controllerchange', () => window.location.reload());

const container = document.getElementById('app-root');
const root = createRoot(container!);
root.render(<App isDevEnv={import.meta.env.DEV} enableUiSW={ENABLE_UI_SW} />);
