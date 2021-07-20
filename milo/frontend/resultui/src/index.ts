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

import { Workbox } from 'workbox-window';

import './stackdriver_errors';
import './routes';
import { NEW_MILO_VERSION_EVENT_TYPE } from './libs/constants';

window.SW_PROMISE = new Promise((resolve) => {
  // Don't cache resources in development mode. Otherwise we will need to
  // refresh the page manually for changes to take effect.
  if (ENABLE_UI_SW && 'serviceWorker' in navigator) {
    const wb = new Workbox('/ui/service-worker.js');
    wb.register().then((registration) => {
      // eslint-disable-next-line no-console
      console.log('UI SW registered: ', registration);

      // Reload the page after a new version is activated.
      navigator.serviceWorker.addEventListener('controllerchange', function (this: ServiceWorkerContainer) {
        if (this.controller!.state === 'activating') {
          this.controller!.addEventListener('statechange', reloadOnceActivated);
        } else {
          reloadOnceActivated.bind(this.controller!)();
        }
      });

      if (registration?.waiting) {
        sendUpdateNotification();
      } else if (registration?.installing) {
        scheduleUpdateNotification.bind(registration)();
      }
      registration?.addEventListener('updatefound', scheduleUpdateNotification);

      resolve(wb);
    });
  }
});

if ('serviceWorker' in navigator) {
  if (!document.cookie.includes('showNewBuildPage=false')) {
    window.addEventListener(
      'load',
      async () => {
        const registration = await navigator.serviceWorker.register('/root-sw.js');
        // eslint-disable-next-line no-console
        console.log('Root SW registered: ', registration);
      },
      { once: true }
    );
  }
}

// Sends an update notification.
function sendUpdateNotification() {
  window.dispatchEvent(new CustomEvent(NEW_MILO_VERSION_EVENT_TYPE));
}

// Sends an update notification once the service worker is installed.
function scheduleUpdateNotification(this: ServiceWorkerRegistration) {
  function onStateChange(this: ServiceWorker) {
    if (this.state === 'installed') {
      sendUpdateNotification();
      this.removeEventListener('statechange', onStateChange);
    }
  }

  this.installing?.addEventListener('statechange', onStateChange);
}

// If the service worker is activated, remove this listener and reload the page.
function reloadOnceActivated(this: ServiceWorker) {
  if (this.state !== 'activated') {
    return;
  }
  this.removeEventListener('statechange', reloadOnceActivated);
  window.location.reload();
}
