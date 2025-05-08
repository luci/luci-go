// Copyright 2025 The LUCI Authors.
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

export interface InfoLineItem {
  label: string;
  value?: string;
  href?: string;
  icon?: JSX.Element;
  isPlaceholder?: boolean;
  customRender?: () => JSX.Element;
}

export const DEFAULT_TOOLTIP_TEXT = 'Copy test name';
export const NO_CLS_TEXT = 'No CLs patched for this test.';
export const NO_ASSOCIATED_BUGS_TEXT = 'No associated bugs found.';
export const NO_HISTORY_DATA_TEXT = 'History data not available.';
export const BISECTION_NO_ANALYSIS_TEXT =
  'No bisection analysis run for this failure.';
export const BISECTION_DATA_INCOMPLETE_TEXT =
  'Bisection data incomplete or not started.';
