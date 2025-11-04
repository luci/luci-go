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

import { AppRoutedTab, AppRoutedTabs } from '@/common/components/routed_tabs';

export function ChroniclePage() {
  return (
    <AppRoutedTabs>
      <AppRoutedTab label="Summary" value="summary" to="summary" />
      <AppRoutedTab label="Timeline" value="timeline" to="timeline" />
      <AppRoutedTab label="Stages & Checks Graph" value="graph" to="graph" />
      <AppRoutedTab label="Tree" value="tree" to="tree" />
      <AppRoutedTab label="Ledger" value="ledger" to="ledger" />
    </AppRoutedTabs>
  );
}

export function Component() {
  return <ChroniclePage />;
}
