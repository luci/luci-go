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

import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Container from '@mui/material/Container';
import Grid from '@mui/material/Grid';
import LinearProgress from '@mui/material/LinearProgress';
import Link from '@mui/material/Link';
import Paper from '@mui/material/Paper';
import { useQuery } from '@tanstack/react-query';
import { useNavigate, useParams } from 'react-router-dom';

import MultiRulesFound from '@/clusters/components/bugs/multi_rules_found/multi_rules_found';
import ErrorAlert from '@/clusters/components/error_alert/error_alert';
import FeedbackSnackbar from '@/clusters/components/error_snackbar/feedback_snackbar';
import LoadErrorAlert from '@/clusters/components/load_error_alert/load_error_alert';
import { SnackbarContextWrapper } from '@/clusters/context/snackbar_context';
import { useRulesService } from '@/clusters/services/services';
import { prpcRetrier } from '@/clusters/tools/prpc_retrier';
import { parseRuleName } from '@/clusters/tools/rules';
import { linkToRule } from '@/clusters/tools/urlHandling/links';
import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { useAuthState } from '@/common/components/auth_state_provider';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { getLoginUrl } from '@/common/tools/url_utils';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { LookupBugRequest } from '@/proto/go.chromium.org/luci/analysis/proto/v1/rules.pb';

export const BugPage = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const service = useRulesService();
  const { identity } = useAuthState();

  const bugSystem = 'buganizer';

  const { isLoading, error, isSuccess, data } = useQuery(
    ['bug', bugSystem, id],
    async () => {
      const request: LookupBugRequest = {
        system: bugSystem,
        // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
        id: id!,
      };
      return await service.LookupBug(request);
    },
    {
      enabled: !!(bugSystem && id),
      retry: prpcRetrier,
    },
  );

  if (!id) {
    return (
      <ErrorAlert
        errorTitle="Bug ID not specified"
        errorText="Bug ID not found in the URL, please check the URL and try again"
        showError
      />
    );
  }

  if (isSuccess && data && data.rules && data.rules.length === 1) {
    const ruleKey = parseRuleName(data.rules[0]);
    const link = linkToRule(ruleKey.project, ruleKey.ruleId);
    // For automatic redirects, replace the history entry in the browser
    // so that if the user clicks 'back', they are not redirected forward
    // again.
    navigate(link, { replace: true });
  }

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
        {isLoading && <LinearProgress />}
        {error && <LoadErrorAlert entityName="rule" error={error} />}
        {isSuccess && data && (
          <Grid container>
            {data.rules ? (
              <MultiRulesFound
                bugSystem={bugSystem}
                bugId={id}
                rules={data.rules}
              />
            ) : (
              <Grid item xs={12}>
                <Alert severity="info" sx={{ mb: 2 }}>
                  <AlertTitle>
                    Could not find a rule for this bug (or you may not have
                    permission to see it)
                  </AlertTitle>
                  {identity === ANONYMOUS_IDENTITY ? (
                    // Because of the design of the RPC, it cannot tell us if there are
                    // rules we do not have access to. If the user is not logged in,
                    // assume a rule exists and prompt the user to log in.
                    <>
                      Please{' '}
                      <Link
                        data-testid="error_login_link"
                        href={getLoginUrl(
                          location.pathname + location.search + location.hash,
                        )}
                      >
                        log in
                      </Link>{' '}
                      to view this information.
                    </>
                  ) : (
                    <>
                      <strong>
                        If you came here from a bug, please check if the bug has
                        been duplicated into another bug and use that bug
                        instead.
                      </strong>
                      &nbsp; It is also possible a rule exists, but you do not
                      have permission to view it.
                    </>
                  )}
                </Alert>
              </Grid>
            )}
          </Grid>
        )}
      </Paper>
    </Container>
  );
};

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="bug">
      <RecoverableErrorBoundary
        // See the documentation in `<LoginPage />` to learn why we handle error
        // this way.
        key="bug"
      >
        <SnackbarContextWrapper>
          <BugPage />
          <FeedbackSnackbar />
        </SnackbarContextWrapper>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
