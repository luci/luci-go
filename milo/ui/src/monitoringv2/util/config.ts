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

import { TreeJson } from './server_json';

// The configured trees that can be monitored.
// TODO: Ideally remove all of this configuration, or at least move it into LUCI config.
export const configuredTrees: TreeJson[] = [
  {
    name: 'android',
    display_name: 'Android',
    bug_queue_label: 'sheriff-android',
    default_monorail_project_name: 'chromium',
    hotlistId: '5562637',
    project: 'chromium',
    treeStatusName: 'chromium',
  },
  {
    name: 'angle',
    display_name: 'Angle',
    default_monorail_project_name: 'angleproject',
    project: 'angle',
    treeStatusName: 'chromium',
  },
  {
    name: 'chrome_browser_release',
    display_name: 'Chrome Browser Release',
    bug_queue_label: 'sheriff-chrome-release',
    default_monorail_project_name: 'chromium',
    hotlistId: '5563460',
    project: 'chromium',
    treeStatusName: 'chromium',
  },
  {
    name: 'chromeos',
    display_name: 'ChromeOS',
    default_monorail_project_name: 'chromium',
    hotlistId: '895366',
    project: 'chromeos',
    treeStatusName: 'chromiumos',
  },
  {
    name: 'chromium',
    display_name: 'Chromium',
    bug_queue_label: 'sheriff-chromium',
    default_monorail_project_name: 'chromium',
    hotlistId: '5563291',
    project: 'chromium',
    treeStatusName: 'chromium',
  },
  {
    name: 'chromium.clang',
    display_name: 'Chromium Clang',
    default_monorail_project_name: 'chromium',
    project: 'chromium',
    treeStatusName: 'chromium',
  },
  {
    name: 'chromium.fuzz',
    display_name: 'Chromium Fuzz',
    project: 'chromium',
    treeStatusName: 'chromium',
  },
  {
    name: 'chromium.gpu',
    display_name: 'Chromium GPU',
    default_monorail_project_name: 'chromium',
    project: 'chromium',
    treeStatusName: 'chromium',
  },
  {
    name: 'chromium.perf',
    display_name: 'Chromium Perf',
    default_monorail_project_name: 'chromium',
    project: 'chromium',
    treeStatusName: 'chromium',
  },
  {
    name: 'dawn',
    display_name: 'Dawn',
    default_monorail_project_name: 'chromium',
    project: 'chromium',
    treeStatusName: 'chromium',
  },
  {
    name: 'fuchsia',
    display_name: 'Fuchsia',
    bug_queue_label: 'sheriff-fuchsia',
    default_monorail_project_name: 'fuchsia',
    project: 'fuchsia',
    treeStatusName: 'fuchsia-stem',
  },
  {
    name: 'ios',
    display_name: 'iOS',
    default_monorail_project_name: 'chromium',
    project: 'chromium',
    treeStatusName: 'chromium',
  },
  {
    name: 'lacros_skylab',
    display_name: 'Lacros Skylab',
    default_monorail_project_name: 'chromium',
    project: 'chromium',
    treeStatusName: 'chromium',
  },
  {
    name: 'devtools_frontend',
    display_name: 'Devtools Frontend',
    hotlistId: '5674718',
    project: 'chromium',
    treeStatusName: 'devtools',
  },
];
