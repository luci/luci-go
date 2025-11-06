// Copyright 2022 The LUCI Authors.
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

import Container from '@mui/material/Container';
import Grid from '@mui/material/Grid2';
import LinearProgress from '@mui/material/LinearProgress';
import { useParams } from 'react-router';

import ClusterAnalysisSection from '@/clusters/components/cluster/cluster_analysis_section/cluster_analysis_section';
import { ClusterContextProvider } from '@/clusters/components/cluster/cluster_context';
import FeedbackSnackbar from '@/clusters/components/error_snackbar/feedback_snackbar';
import LoadErrorAlert from '@/clusters/components/load_error_alert/load_error_alert';
import RuleArchivedMessage from '@/clusters/components/rule/rule_archived_message/rule_archived_message';
import RuleTopPanel from '@/clusters/components/rule/rule_top_panel/rule_top_panel';
import { SnackbarContextWrapper } from '@/clusters/context/snackbar_context';
import useFetchRule from '@/clusters/hooks/use_fetch_rule';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useDeclarePageId } from '@/common/components/page_meta';
import { UiPage } from '@/common/constants/view';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';

export const RulePage = () => {
  const { project, id } = useParams();
  if (!project || !id) {
    throw new Error('invariant violated: project, id must be set');
  }

  const {
    isPending,
    data: rule,
    error,
    isSuccess,
  } = useFetchRule(project || '', id || '');

  return (
    <Container className="mt-1" maxWidth={false}>
      <title>{`Rule | ${id}`}</title>
      <Grid sx={{ mt: 1 }} container spacing={2}>
        {isPending && <LinearProgress />}
        {error && <LoadErrorAlert entityName="rule" error={error} />}
        {isSuccess && rule && (
          <>
            <Grid size={12}>
              <RuleTopPanel project={project} ruleId={id} />
            </Grid>
            <Grid size={12}>
              {rule?.isActive ? (
                <ClusterContextProvider
                  project={project}
                  clusterAlgorithm="rules"
                  clusterId={id}
                >
                  <ClusterAnalysisSection />
                </ClusterContextProvider>
              ) : (
                <RuleArchivedMessage />
              )}
            </Grid>
          </>
        )}
      </Grid>
    </Container>
  );
};

export function Component() {
  useDeclarePageId(UiPage.Rules);

  return (
    <TrackLeafRoutePageView contentGroup="rule">
      <RecoverableErrorBoundary
        // See the documentation in `<LoginPage />` to learn why we handle error
        // this way.
        key="rule"
      >
        <SnackbarContextWrapper>
          <RulePage />
          <FeedbackSnackbar />
        </SnackbarContextWrapper>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
