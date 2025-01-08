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

import { GrpcError, RpcCode } from '@chopsui/prpc-client';
import LoadingButton from '@mui/lab/LoadingButton';
import Backdrop from '@mui/material/Backdrop';
import CircularProgress from '@mui/material/CircularProgress';
import Container from '@mui/material/Container';
import Grid from '@mui/material/Grid2';
import Paper from '@mui/material/Paper';
import Typography from '@mui/material/Typography';
import { useMutation } from '@tanstack/react-query';
import { ChangeEvent, useContext, useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

import BugPicker from '@/clusters/components/bug_picker/bug_picker';
import ErrorAlert from '@/clusters/components/error_alert/error_alert';
import FeedbackSnackbar from '@/clusters/components/error_snackbar/feedback_snackbar';
import PanelHeading from '@/clusters/components/headings/panel_heading/panel_heading';
import RuleEditInput from '@/clusters/components/rule_edit_input/rule_edit_input';
import {
  SnackbarContext,
  SnackbarContextWrapper,
} from '@/clusters/context/snackbar_context';
import { useRulesService } from '@/clusters/services/services';
import { linkToRule } from '@/clusters/tools/urlHandling/links';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { PageMeta, usePageId, useProject } from '@/common/components/page_meta';
import { UiPage } from '@/common/constants/view';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { ClusterId } from '@/proto/go.chromium.org/luci/analysis/proto/v1/common.pb';
import {
  CreateRuleRequest,
  Rule,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/rules.pb';

export const NewRulePage = () => {
  const { project } = useParams();
  if (!project) {
    throw new Error('invariant violated: project must be set');
  }
  useProject(project);

  const navigate = useNavigate();
  const [searchParams] = useSyncedSearchParams();

  const [bugId, setBugId] = useState<string>('');
  const [definition, setDefinition] = useState<string>('');
  const [sourceCluster, setSourceCluster] = useState<ClusterId>({
    algorithm: '',
    id: '',
  });
  const service = useRulesService();

  const { setSnack } = useContext(SnackbarContext);
  const createRule = useMutation((request: CreateRuleRequest) =>
    service.Create(request),
  );
  const [validationError, setValidationError] = useState<GrpcError | null>(
    null,
  );

  useEffect(() => {
    const rule = searchParams.get('rule');
    if (rule) {
      setDefinition(rule);
    }
    const sourceClusterAlg = searchParams.get('sourceAlg');
    const sourceClusterID = searchParams.get('sourceId');
    if (sourceClusterAlg && sourceClusterID) {
      setSourceCluster({
        algorithm: sourceClusterAlg,
        id: sourceClusterID,
      });
    }
  }, [searchParams]);

  const handleBugIdChange = (bugId: string) => {
    setBugId(bugId);
  };

  const handleRuleDefinitionChange = (e: ChangeEvent<HTMLTextAreaElement>) => {
    setDefinition(e.target.value);
  };

  const handleSave = () => {
    const request: CreateRuleRequest = {
      parent: `projects/${project}`,
      rule: Rule.create({
        bug: {
          system: 'buganizer',
          id: bugId,
        },
        ruleDefinition: definition,
        isActive: true,
        isManagingBug: false,
        isManagingBugPriority: true,
        sourceCluster: sourceCluster,
      }),
    };
    createRule.mutate(request, {
      onSuccess(data) {
        const rule = data;
        setSnack({
          open: true,
          message: 'Rule updated successfully',
          severity: 'success',
        });
        navigate(linkToRule(rule.project, rule.ruleId));
      },
      onError(error) {
        if (
          error instanceof GrpcError &&
          error.code === RpcCode.INVALID_ARGUMENT
        ) {
          setValidationError(error);
        } else {
          setSnack({
            open: true,
            message: `Failed to create rule due to: ${error}`,
            severity: 'error',
          });
        }
      },
    });
  };

  return (
    <Container>
      <Paper
        elevation={3}
        sx={{
          pt: 1,
          pb: 4,
          px: 2,
          mt: 1,
          mx: 2,
        }}
      >
        <PageMeta title="New Rule" />
        <PanelHeading>New Rule</PanelHeading>
        <Grid container direction="column" spacing={1}>
          <Grid size="grow">
            {validationError && (
              <ErrorAlert
                errorTitle="Validation error"
                errorText={`Rule data is invalid: ${validationError.description.trim()}`}
                showError
                onErrorClose={() => setValidationError(null)}
              />
            )}
          </Grid>
          <Grid container size="grow">
            <Grid size={6}>
              <Typography>Associated bug</Typography>
              <BugPicker bugId={bugId} handleBugIdChanged={handleBugIdChange} />
            </Grid>
          </Grid>
          <Grid marginTop="1rem" size="grow">
            <Typography>Rule definition</Typography>
            <RuleEditInput
              definition={definition}
              onDefinitionChange={handleRuleDefinitionChange}
            />
          </Grid>
          <Grid size="grow">
            <LoadingButton
              variant="contained"
              data-testid="create-rule-save"
              onClick={handleSave}
              loading={createRule.isLoading}
            >
              Save
            </LoadingButton>
          </Grid>
        </Grid>
      </Paper>
      <Backdrop sx={{ color: '#fff' }} open={createRule.isSuccess}>
        <CircularProgress color="inherit" />
      </Backdrop>
    </Container>
  );
};

export function Component() {
  usePageId(UiPage.Rules);

  return (
    <TrackLeafRoutePageView contentGroup="new-rule">
      <RecoverableErrorBoundary
        // See the documentation in `<LoginPage />` to learn why we handle error
        // this way.
        key="new-rule"
      >
        <SnackbarContextWrapper>
          <NewRulePage />
          <FeedbackSnackbar />
        </SnackbarContextWrapper>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
