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
 * Version of the UI.
 * Declared in the server generated file, /ui_version.js, included as a script
 * tag.
 */
declare const UI_VERSION: string;

/**
 * Type of the UI.
 *
 * Possible values are:
 *  * 'new-ui': for regular UI.
 *  * 'old-ui': when user has activated USER_INITIATED_ROLLBACK.
 *
 * Declared in the server generated file, /ui_version.js, included as a script
 * tag.
 */
declare const UI_VERSION_TYPE: 'new-ui' | 'old-ui';

/**
 * Settings of the app.
 * Declared in the server generated file, /settings.js, included as a script
 * tag.
 */
// While it's possible to use a code generated binding for this. The JSON
// representation of a proto message does not necessarily match the generated
// type of the same proto message, particularly when timestamp, duration, enums,
// etc are used. Keep this as a hand written binding so we don't need to deal
// with the complexity of adding a JSON -> JS converter for the settings, which
// is used in places (e.g. a worker script) that don't want heavy dependencies
// like a proto message parser.
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
    /**
     * Host to use for pRPC APIs.
     */
    readonly host: string;
    /**
     * Host to use for links to UI.
     */
    readonly uiHost: string;
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
  readonly luciNotify: {
    readonly host: string;
  };
  readonly authService: {
    readonly host: string;
  };
  readonly crRev: {
    readonly host: string;
  };
  readonly milo: {
    readonly host: string;
  };
  readonly luciSourceIndex: {
    readonly host: string;
  };
  readonly fleetConsole: {
    readonly host: string;
    readonly hats: HaTSConfig;
    readonly enableColumnFilter: boolean;
  };
  readonly ufs: {
    readonly host: string;
  };
  readonly testInvestigate: {
    // To store all trigger IDs for different HaTS surveys on the test investigate page.
    readonly hatsPositiveRecs: HaTSConfig;
    readonly hatsNegativeRecs: HaTSConfig;
    readonly hatsCUJ: HaTSConfig;
  };
};

/**
 * Import all JS modules. This ensures all modules are initialized.
 */
declare function preloadModules(): void;
