/* Copyright 2020 The LUCI Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

type Constructor<T, P extends unknown[] = []> = new (...params: P) => T;

type Mutable<T> = {
  -readonly [key in keyof T]: T[key];
};

type DeepMutable<T> = {
  -readonly [key in keyof T]: DeepMutable<T[key]>;
};

interface ToString {
  toString(): string;
}

type Result<T, E> = ResultOk<T> | ResultErr<E>;

interface ResultOk<T> {
  ok: true;
  value: T;
}

interface ResultErr<E> {
  ok: false;
  value: E;
}

/**
 * Configs of the app.
 * Declared in the server generated file, /configs.js, included as a script tag.
 */
declare const CONFIGS: {
  readonly RESULT_DB: {
    readonly HOST: string;
  };
  readonly BUILDBUCKET: {
    readonly HOST: string;
  };
  readonly WEETBIX: {
    readonly HOST: string;
  };
};

/**
 * Whether GA tracking should be enabled.
 * Injected by webpack.
 */
declare const ENABLE_GA: boolean;

/**
 * Whether the UI service worker should be enabled.
 */
declare const ENABLE_UI_SW: boolean;

/**
 * A string that is unique per page load;
 */
declare const VISIT_ID: string;

/**
 * A promise that returns the ui service worker.
 */
// eslint-disable-next-line no-var
declare var SW_PROMISE: Promise<import('workbox-window').Workbox>;

/**
 * A cached user ID loaded from the service worker.
 */
declare const CACHED_USER_ID: string | null;

/**
 * The start time of the page visit.
 */
declare const TIME_ORIGIN: number;
