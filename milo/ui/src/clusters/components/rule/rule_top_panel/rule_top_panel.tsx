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

import Grid from '@mui/material/Grid';
import LinearProgress from '@mui/material/LinearProgress';

import ErrorAlert from '@/clusters/components/error_alert/error_alert';
import ReclusteringProgressIndicator from '@/clusters/components/reclustering_progress_indicator/reclustering_progress_indicator';
import RuleInfo from '@/clusters/components/rule/rule_info/rule_info';
import TimestampInfoBar from '@/clusters/components/timestamp_info_bar/timestamp_info_bar';
import useFetchRule from '@/clusters/hooks/use_fetch_rule';

import BugInfo from '../bug_info/bug_info';

interface Props {
  project: string;
  ruleId: string;
}

const RuleTopPanel = ({ project, ruleId }: Props) => {
  const {
    isLoading,
    isError,
    data: rule,
    error,
  } = useFetchRule(project, ruleId);

  if (isLoading) {
    return <LinearProgress />;
  }

  if (isError) {
    return (
      <ErrorAlert
        errorText={`An error occured while fetching the rule: ${error}`}
        errorTitle="Failed to load rule"
        showError
      />
    );
  }

  return (
    <>
      {rule && (
        <Grid container columnSpacing={2}>
          <Grid item xs={12}>
            <ReclusteringProgressIndicator
              hasRule={true}
              project={project}
              rulePredicateLastUpdated={rule.predicateLastUpdateTime}
            />
          </Grid>
          <Grid item xs={12}>
            <TimestampInfoBar
              createUsername={rule.createUser}
              createTime={rule.createTime}
              updateUsername={rule.lastAuditableUpdateUser}
              updateTime={rule.lastAuditableUpdateTime}
            />
          </Grid>
          <Grid container item xs={12} columnSpacing={2}>
            <Grid item xs={12} lg={8} display="grid">
              <RuleInfo project={project} rule={rule} />
            </Grid>
            <Grid item xs={12} lg={4} display="grid">
              <BugInfo rule={rule} />
            </Grid>
          </Grid>
        </Grid>
      )}
    </>
  );
};

export default RuleTopPanel;
