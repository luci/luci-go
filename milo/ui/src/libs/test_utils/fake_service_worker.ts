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
 * A fake service worker with none of its method implemented.
 * The method should be stubbed according to the test requirement.
 * We cannot stub `ServiceWorker` because it doesn't have a legal constructor.
 */
export class FakeServiceWorker implements ServiceWorker {
  constructor(public state: ServiceWorkerState, public scriptURL: string) {}

  onstatechange: ((this: ServiceWorker, ev: Event) => any) | null = null;

  postMessage(message: any, transfer: Transferable[]): void;
  postMessage(message: any, options?: StructuredSerializeOptions | undefined): void;
  postMessage(_message: unknown, _options?: unknown): void {
    throw new Error('Method not implemented.');
  }

  addEventListener<K extends keyof ServiceWorkerEventMap>(
    type: K,
    listener: (this: ServiceWorker, ev: ServiceWorkerEventMap[K]) => any,
    options?: boolean | AddEventListenerOptions | undefined
  ): void;
  addEventListener(
    type: string,
    listener: EventListenerOrEventListenerObject,
    options?: boolean | AddEventListenerOptions | undefined
  ): void;
  addEventListener(_type: string, _listener: unknown, _options?: unknown): void {
    throw new Error('Method not implemented.');
  }

  removeEventListener<K extends keyof ServiceWorkerEventMap>(
    type: K,
    listener: (this: ServiceWorker, ev: ServiceWorkerEventMap[K]) => any,
    options?: boolean | EventListenerOptions | undefined
  ): void;
  removeEventListener(
    type: string,
    listener: EventListenerOrEventListenerObject,
    options?: boolean | EventListenerOptions | undefined
  ): void;
  removeEventListener(_type: string, _listener: unknown, _options?: unknown): void {
    throw new Error('Method not implemented.');
  }

  dispatchEvent(event: Event): boolean;
  dispatchEvent(_event: unknown): boolean {
    throw new Error('Method not implemented.');
  }

  onerror: ((this: AbstractWorker, ev: ErrorEvent) => any) | null = null;
}
