// Copyright 2020 The LUCI Authors.
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

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { AppRoutedTabs, AppRoutedTab } from '@/common/components/routed_tabs';

export function AnalysesPage() {
  return (
    <AppRoutedTabs>
      <AppRoutedTab
        label="Compile Analysis"
        value="compile-analysis"
        to="compile-analysis"
      />
      <AppRoutedTab
        label="Test Analysis"
        value="test-analysis"
        to="test-analysis"
      />
    </AppRoutedTabs>
  );
}

export const element = (
  // See the documentation for `<LoginPage />` for why we handle error this way.
  <RecoverableErrorBoundary key="analyses-page">
    <AnalysesPage />
  </RecoverableErrorBoundary>
);
