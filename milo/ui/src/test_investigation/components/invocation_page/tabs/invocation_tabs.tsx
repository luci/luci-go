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

import { Box } from '@mui/material';

import { RoutedTab, RoutedTabs } from '@/generic_libs/components/routed_tabs';
import { useInvocation } from '@/test_investigation/context/context';
import { isRootInvocation } from '@/test_investigation/utils/invocation_utils';

export function InvocationTabs() {
  const invocation = useInvocation();
  const hasSummary =
    isRootInvocation(invocation) && !!invocation.summaryMarkdown;

  return (
    <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
      <RoutedTabs sx={{ mb: 1 }}>
        <RoutedTab label="Tests" value="tests" to="tests" />
        {hasSummary && (
          <RoutedTab label="Summary" value="summary" to="summary" />
        )}
        <RoutedTab label="Details" value="details" to="details" />
      </RoutedTabs>
    </Box>
  );
}
