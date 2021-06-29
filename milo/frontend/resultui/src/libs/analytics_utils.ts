// Copyright 2021 The LUCI Authors.
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

export enum GA_CATEGORIES {
  NEW_BUILD_PAGE = 'New Build Page',
  OVERVIEW_TAB = 'Overview Tab',
  TEST_RESULTS_TAB = 'Test Results Tab',
  STEPS_TAB = 'Steps Tab',
  RELATED_BUILD_TAB = 'Related Builds Tab',
  TIMELINE_TAB = 'Timeline Tab',
  BLAMELIST_TAB = 'Blamelist Tab',
  STEP_WITH_UNEXPECTED_RESULTS = 'Step with Unexpected Results',
  TEST_VARIANT_WITH_UNEXPECTED_RESULTS = 'Test Variant with Unexpected Results',
  LEGACY_BUILD_PAGE = 'Legacy Build Page',
  PROJECT_BUILD_PAGE = 'Project Build Page',
}

export enum GA_ACTIONS {
  SWITCH_VERSION = 'Switch Version',
  SWITCH_VERSION_TEMP = 'Switch Version Temporarily',
  PAGE_VISITED = 'Page Visited',
  TAB_VISITED = 'Tab Visited',
  LOADING_TIME = 'Loading Time',
  EXPAND_ENTRY = 'Expand Entry',
  INSPECT_TEST = 'Inspect Test',
  VISITED_LEGACY = 'Visited Legacy',
  VISITED_NEW = 'Visited New',
}

export function trackEvent(category: GA_CATEGORIES, action: GA_ACTIONS, label?: string, value?: number) {
  if (!ENABLE_GA) {
    return;
  }

  // See https://developers.google.com/analytics/devguides/collection/analyticsjs/events?hl=en#event_fields
  // for more information
  window.ga('send', {
    hitType: 'event',
    eventCategory: category,
    eventAction: action,
    eventLabel: label,
    eventValue: value,
    transport: 'beacon',
  });
}

// Sometimes we need to have a unique label in order to access to the raw value
// data. Otherwise, value data will be aggregated into categories/actions
export function generateRandomLabel(prefix: string): string {
  return prefix + '_' + Math.random().toString(36).substr(2, 8);
}
