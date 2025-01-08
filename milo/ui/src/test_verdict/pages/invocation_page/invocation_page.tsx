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

import { LinearProgress } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { Helmet } from 'react-helmet';
import { useParams } from 'react-router-dom';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useProject } from '@/common/components/page_meta';
import { AppRoutedTab, AppRoutedTabs } from '@/common/components/routed_tabs';
import { ContentGroup } from '@/generic_libs/components/google_analytics';
import { useResultDbClient } from '@/test_verdict/hooks/prpc_clients';

import { InvocationIdBar } from './invocation_id_bar';
import { VerdictCountIndicator } from './verdict_count_indicator';

// TODO(b/40253769): replace the `@/test_verdict/legacy/invocation_page` with
// this implementation once we migrated the test results tab to React
// (therefore no longer depends on the MobB store).
export function InvocationPage() {
  const { invId } = useParams();
  if (!invId) {
    throw new Error('invariant violated: invId must be set');
  }

  const invName = 'invocations/' + invId;

  const client = useResultDbClient();
  const { data, error, isError, isLoading } = useQuery(
    client.GetInvocation.query({ name: invName }),
  );
  if (isError) {
    throw error;
  }

  const project = data?.realm.split(':', 2)[0] || '';
  useProject(project);

  return (
    <>
      <Helmet>
        <title>inv: {invId}</title>
      </Helmet>
      <InvocationIdBar invName={invName} />
      <LinearProgress
        value={100}
        variant={isLoading ? 'indeterminate' : 'determinate'}
      />
      <AppRoutedTabs>
        <AppRoutedTab
          label="Test Results"
          value="test-results"
          to="test-results"
          icon={<VerdictCountIndicator invName={invName} />}
          iconPosition="end"
        />
        <AppRoutedTab
          label="Invocation Details"
          value="invocation-details"
          to="invocation-details"
        />
      </AppRoutedTabs>
    </>
  );
}

export function Component() {
  return (
    <ContentGroup group="invocation">
      <RecoverableErrorBoundary
        // See the documentation in `<LoginPage />` to learn why we handle error
        // this way.
        key="invocation"
      >
        <InvocationPage />
      </RecoverableErrorBoundary>
    </ContentGroup>
  );
}
