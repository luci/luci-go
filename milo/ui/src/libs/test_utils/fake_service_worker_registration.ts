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

/* eslint-disable @typescript-eslint/no-explicit-any */

/**
 * A fake service worker registration with none of its method implemented.
 * The method should be stubbed according to the test requirement.
 * We cannot stub `ServiceWorkerRegistration` because it doesn't have a legal
 * constructor.
 */
export class FakeServiceWorkerRegistration implements ServiceWorkerRegistration {
  active: ServiceWorker | null = null;
  installing: ServiceWorker | null = null;
  waiting: ServiceWorker | null = null;
  updateViaCache: ServiceWorkerUpdateViaCache = 'none';
  onupdatefound: ((this: ServiceWorkerRegistration, ev: Event) => any) | null = null;

  get navigationPreload(): NavigationPreloadManager {
    throw new Error('Method not implemented.');
  }
  get pushManager(): PushManager {
    throw new Error('Method not implemented.');
  }

  constructor(public scope: string) {}

  getNotifications(filter?: GetNotificationOptions | undefined): Promise<Notification[]>;
  getNotifications(_filter?: unknown): Promise<Notification[]> {
    throw new Error('Method not implemented.');
  }

  showNotification(title: string, options?: NotificationOptions | undefined): Promise<void>;
  showNotification(_title: unknown, _options?: unknown): Promise<void> {
    throw new Error('Method not implemented.');
  }

  unregister(): Promise<boolean>;
  unregister(): Promise<boolean> {
    throw new Error('Method not implemented.');
  }

  update(): Promise<void>;
  update(): Promise<void> {
    throw new Error('Method not implemented.');
  }

  addEventListener<K extends 'updatefound'>(
    type: K,
    listener: (this: ServiceWorkerRegistration, ev: ServiceWorkerRegistrationEventMap[K]) => any,
    options?: boolean | AddEventListenerOptions | undefined
  ): void;
  addEventListener(
    type: string,
    listener: EventListenerOrEventListenerObject,
    options?: boolean | AddEventListenerOptions | undefined
  ): void;
  addEventListener(_type: unknown, _listener: unknown, _options?: unknown): void {
    throw new Error('Method not implemented.');
  }

  removeEventListener<K extends 'updatefound'>(
    type: K,
    listener: (this: ServiceWorkerRegistration, ev: ServiceWorkerRegistrationEventMap[K]) => any,
    options?: boolean | EventListenerOptions | undefined
  ): void;
  removeEventListener(
    type: string,
    listener: EventListenerOrEventListenerObject,
    options?: boolean | EventListenerOptions | undefined
  ): void;
  removeEventListener(_type: unknown, _listener: unknown, _options?: unknown): void {
    throw new Error('Method not implemented.');
  }

  dispatchEvent(event: Event): boolean;
  dispatchEvent(_event: unknown): boolean {
    throw new Error('Method not implemented.');
  }
}
