// Copyright 2026 The LUCI Authors.
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
 * Generates deterministic-looking but randomized metric rows for a given array of string values.
 * This ensures the UI has data to render for the initial layout testing phase.
 */

function createMockRows(values: string[]): Record<string, number | string>[] {
  return values.map((val) => ({
    dimension_value: val,
    COUNT: Math.floor(Math.random() * 50) + 10,
    MIN: Math.random() * 20 + 5,
    MAX: Math.random() * 50 + 100,
    MEAN: Math.random() * 30 + 40,
    P50: Math.random() * 30 + 35,
    P99: Math.random() * 40 + 80,
  }));
}

/**
 * Mock table payload used solely for the Crystal Ball demo page.
 * // TODO: This is just temporary until the data is plumbed through.
 */
export const MOCK_BREAKDOWN_DATA = {
  sections: [
    {
      dimensionColumn: 'ATP_Test_Name',
      rows: createMockRows([
        'com.android.launcher3.tests.LauncherJankTests#testAppLaunch',
        'android.platform.test.scenario.sysui.NotificationShadeScenario#testClearAll',
        'com.android.systemui.tests.bubbles.BubbleStackViewTest#testBubbleDrag',
      ]),
    },
    {
      dimensionColumn: 'Test_Module',
      rows: createMockRows([
        'LauncherJankTests',
        'SystemUITests',
        'SettingsPerfTests',
        'FrameworksServicesTests',
      ]),
    },
    {
      dimensionColumn: 'Test_Class',
      rows: createMockRows([
        'LauncherJankTests',
        'NotificationShadeScenario',
        'BubbleStackViewTest',
        'WindowManagerServiceTest',
      ]),
    },
    {
      dimensionColumn: 'Test_Method',
      rows: createMockRows([
        'testAppLaunch',
        'testRelayoutWindow',
        'testClearAll',
        'testBubbleDrag',
      ]),
    },
    {
      dimensionColumn: 'Full_Test_Name',
      rows: createMockRows([
        'com.android.launcher3.tests.LauncherJankTests#testAppLaunch',
        'FrameworksServicesTests:com.android.server.wm.WindowManagerServiceTest#testRelayoutWindow',
        'android.platform.test.scenario.sysui.NotificationShadeScenario#testClearAll',
      ]),
    },
    {
      dimensionColumn: 'Build_Branch',
      rows: createMockRows(['git_main', 'tm-qpr-dev', 'udc-dev', 'vic-dev']),
    },
    {
      dimensionColumn: 'Build_Target',
      rows: createMockRows([
        'oriole-userdebug',
        'raven-userdebug',
        'tangorpro-userdebug',
        'husky-userdebug',
      ]),
    },
    {
      dimensionColumn: 'Model',
      rows: createMockRows([
        'Pixel 5',
        'Pixel 5 Pro',
        'Pixel 6',
        'Pixel 6 Pro',
        'Pixel 7a',
        'Pixel 7 Pro',
        'Pixel 8',
        'Pixel 8a',
        'Pixel 8 Pro',
        'Pixel 9',
        'Pixel 9a',
        'Pixel 9 Pro',
        'Pixel Fold',
        'Pixel Fold 2',
        'Pixel Fold 3',
        'Pixel Tablet',
      ]),
    },
    {
      dimensionColumn: 'SKU',
      rows: createMockRows(['G9S9B', 'GLU0G', 'GTU8P', 'GC3VE']),
    },
    {
      dimensionColumn: 'Board',
      rows: createMockRows(['oriole', 'raven', 'tangorpro', 'husky']),
    },
  ],
};
