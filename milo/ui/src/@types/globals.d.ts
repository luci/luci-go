// Copyright 2023 The LUCI Authors.
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

/**
 * Version of the app.
 * Declared in the server generated file, /configs.js, included as a script tag.
 */
declare const VERSION: string;

/**
 * Settings of the app.
 * Declared in the server generated file, /configs.js, included as a script tag.
 */
declare const SETTINGS: {
  readonly buildbucket: {
    readonly host: string;
  };
  readonly swarming: {
    readonly defaultHost: string;
    readonly allowedHosts?: readonly string[];
  };
  readonly resultdb: {
    readonly host: string;
  };
  readonly luciAnalysis: {
    readonly host: string;
  };
  readonly luciBisection: {
    readonly host: string;
  };
  readonly sheriffOMatic: {
    readonly host: string;
  };
  readonly luciTreeStatus: {
    readonly host: string;
  };
};

/**
 * Google Analytics interfaces.
 */
interface GAArgs {
  hitType: string;
  eventCategory: string;
  eventAction: string;
  eventLabel: string;
  transport: string;
}
declare const ga: (operation: string, args: GAArgs) => void;
