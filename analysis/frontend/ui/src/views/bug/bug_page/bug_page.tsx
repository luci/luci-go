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

import { useQuery } from 'react-query';
import {
  useNavigate,
  useParams,
} from 'react-router-dom';

import Container from '@mui/material/Container';
import Grid from '@mui/material/Grid';
import LinearProgress from '@mui/material/LinearProgress';
import Paper from '@mui/material/Paper';

import MultiRulesFound from '@/components/bugs/multii_rules_found/multi_rules_found';
import ErrorAlert from '@/components/error_alert/error_alert';
import LoadErrorAlert from '@/components/load_error_alert/load_error_alert';
import {
  getRulesService,
  LookupBugRequest,
  parseRuleName,
} from '@/services/rules';
import { linkToRule } from '@/tools/urlHandling/links';
import { prpcRetrier } from '@/services/shared_models';

const BugPage = () => {
  const { bugTracker, id } = useParams();
  const navigate = useNavigate();

  let bugSystem = '';
  let bugId: string | undefined = '';

  if (bugTracker == 'b') {
    bugSystem = 'buganizer';
    bugId = id;
  } else {
    bugSystem = 'monorail';
    bugId = bugTracker + '/' + id;
  }

  const {
    isLoading,
    error,
    isSuccess,
    data,
  } = useQuery(['bug', bugSystem, bugId], async () => {
    const service = getRulesService();
    const request: LookupBugRequest = {
      system: bugSystem,
      // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
      id: bugId!,
    };
    return await service.lookupBug(request);
  }, {
    enabled: !!(bugSystem && bugId),
    retry: prpcRetrier,
  });

  if (!bugTracker || !bugId) {
    return (
      <ErrorAlert
        errorTitle="Bug tracker or bug ID not specified"
        errorText="Bug tracker or bug ID not found in the URL, please check the URL and try again"
        showError
      />
    );
  }

  if (isSuccess &&
    data &&
    data.rules &&
    data.rules.length === 1) {
    const ruleKey = parseRuleName(data.rules[0]);
    const link = linkToRule(ruleKey.project, ruleKey.ruleId);
    navigate(link);
  }

  return (
    <Container>
      <Paper elevation={3} sx={{
        pt: 1,
        pb: 4,
        px: 2,
        mt: 1,
        mx: 2,
      }}>
        {isLoading && (
          <LinearProgress />
        )}
        {
          error && (
            <LoadErrorAlert
              entityName="bug"
              error={error}
            />
          )
        }
        {
          isSuccess && data && (
            <Grid container>
              {
                data.rules ? (
                  <MultiRulesFound
                    bugSystem={bugSystem}
                    bugId={bugId}
                    rules={data.rules}
                  />
                ) : (
                  <Grid item xs={12}>
                    <ErrorAlert
                      errorTitle="Rule not found"
                      errorText={`No rule found matching the specified bug (${bugSystem}:${bugId}).`}
                      showError
                    />
                  </Grid>
                )
              }
            </Grid>
          )
        }
      </Paper>
    </Container>
  );
};

export default BugPage;
