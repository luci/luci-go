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

import { Workbox } from 'workbox-window';

import { createStaticTrustedURL } from '@/generic_libs/tools/utils';

export interface InitUiSwOptions {
  readonly isDevEnv: boolean;
}

export function initUiSW({ isDevEnv }: InitUiSwOptions) {
  // vite-plugin-pwa hosts the service worker in a different route in dev
  // mode.
  // See https://vite-pwa-org.netlify.app/guide/development.html#injectmanifest-strategy
  const uiSwUrl = isDevEnv ? '/ui/dev-sw.js?dev-sw' : '/ui/ui_sw.js';
  const workbox = new Workbox(
    createStaticTrustedURL('ui-sw-js-static', uiSwUrl),
    // During development, the service worker script can only be a JS module,
    // because it runs through the same pipeline as the rest of the scripts.
    // In production, the service worker script cannot be a JS module due to
    // limited browser support.
    { type: isDevEnv ? 'module' : 'classic' },
  );

  workbox.register().then((r) => {
    // When the service worker is being updated, load all the lazy loaded
    // modules. This prevents errors when navigating to a view that requires an
    // outdated and purged JS module.
    //
    // It's possible that the service worker clears the caches before all
    // modules are loaded. This is not a big issue since:
    // 1. Users may not navigate to the page that requires the module.
    // 2. The error can be fixed by refreshing the page.
    // 3. This should be rare since all modules should be served from the cache.
    // 4. If this happens frequently, we can add a small delay before activating
    //    the service worker.
    function preloadLazyRoutes() {
      preloadModules();
      r?.removeEventListener('updatefound', preloadLazyRoutes);
    }
    r?.addEventListener('updatefound', preloadLazyRoutes);
  });
}
