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

import { useState } from 'react';
import { useQuery } from 'react-query';

import Edit from '@mui/icons-material/Edit';
import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import CircularProgress from '@mui/material/CircularProgress';
import Container from '@mui/material/Container';
import Divider from '@mui/material/Divider';
import Grid from '@mui/material/Grid';
import IconButton from '@mui/material/IconButton';
import LinearProgress from '@mui/material/LinearProgress';
import Link from '@mui/material/Link';
import Paper from '@mui/material/Paper';
import Switch from '@mui/material/Switch';
import PanelHeading from '@/components/headings/panel_heading/panel_heading';

import { useMutateRule } from '@/hooks/use_mutate_rule';
import {
  GetIssueRequest,
  getIssuesService,
} from '@/legacy_services/monorail';
import {
  Rule,
  UpdateRuleRequest,
} from '@/legacy_services/rules';
import {
  AssociatedBug,
  prpcRetrier,
} from '@/legacy_services/shared_models';
import { MuiDefaultColor } from '@/types/mui_types';
import GridLabel from '@/components/grid_label/grid_label';
import HelpTooltip from '@/components/help_tooltip/help_tooltip';
import LoadErrorAlert from '@/components/load_error_alert/load_error_alert';
import BugEditDialog from '@/components/rule/bug_edit_dialog/bug_edit_dialog';

const createIssueServiceRequest = (bug: AssociatedBug): GetIssueRequest => {
  const parts = bug.id.split('/');
  const monorailProject = parts[0];
  const bugId = parts[1];
  const issueId = `projects/${monorailProject}/issues/${bugId}`;
  return {
    name: issueId,
  };
};

const bugStatusColor = (status: string): MuiDefaultColor => {
  // In monorail, bug statuses are configurable per system. Right now,
  // we don't have a configurable mapping from status to semantic in
  // LUCI Analysis. We will try to recognise common terminology and fall
  // back to "other" status otherwise.
  status = status.toLowerCase();
  const unassigned = ['new', 'untriaged', 'available'];
  const assigned = ['accepted', 'assigned', 'started', 'externaldependency'];
  const fixed = ['fixed', 'verified'];
  if (unassigned.indexOf(status) >= 0) {
    return 'error';
  } else if (assigned.indexOf(status) >= 0) {
    return 'primary';
  } else if (fixed.indexOf(status) >= 0) {
    return 'success';
  } else {
    // E.g. Won't fix, duplicate, archived.
    return 'info';
  }
};

const bugUpdatesHelpText = 'Whether the associated bug should be automatically verified (or re-opened)' +
                          ' based on cluster impact. Only one rule may be set to' +
                          ' update a given bug at any one time.';

const bugPriorityUpdateHelpText = 'Whether the priority of the associated bug should be' +
    ' automatically updated based on cluster impact.';

interface Props {
    rule: Rule;
}

const BugInfo = ({
  rule,
}: Props) => {
  const issueService = getIssuesService();

  const [editDialogOpen, setEditDialogOpen] = useState(false);

  const isMonorail = (rule.bug.system == 'monorail');
  const requestName = rule.bug.system + '/' + rule.bug.id;
  const { isLoading, data: issue, error } = useQuery(['bug', requestName],
      async () => {
        const fetchBugRequest = createIssueServiceRequest(rule.bug);
        return await issueService.getIssue(fetchBugRequest);
      }, {
        retry: prpcRetrier,
        enabled: isMonorail,
      },
  );

  const mutateRule = useMutateRule();

  const handleToggleUpdateBug = () => {
    const request: UpdateRuleRequest = {
      rule: {
        name: rule.name,
        isManagingBug: !rule.isManagingBug,
      },
      updateMask: 'isManagingBug',
      etag: rule.etag,
    };
    mutateRule.mutate(request);
  };

  const handleToggleUpdateBugPriority = () => {
    const request: UpdateRuleRequest = {
      rule: {
        name: rule.name,
        isManagingBugPriority: !rule.isManagingBugPriority,
      },
      updateMask: 'isManagingBugPriority',
      etag: rule.etag,
    };
    mutateRule.mutate(request);
  };

  return (
    <Paper data-cy="bug-info" elevation={3} sx={{ pt: 2, pb: 2, mt: 1 }}>
      <Container maxWidth={false}>
        <PanelHeading>
          Associated Bug
        </PanelHeading>
        <Grid container rowGap={0}>
          <GridLabel xs={4} lg={2} text="Bug">
          </GridLabel>
          <Grid container item xs={8} lg={5} alignItems="center" columnGap={1}>
            <Link data-testid="bug" target="_blank" href={rule.bug.url}>
              {rule.bug.linkText}
            </Link>
            <IconButton data-testid="bug-edit" aria-label="edit" onClick={() => setEditDialogOpen(true)}>
              <Edit />
            </IconButton>
          </Grid>
          <GridLabel xs={4} lg={3} text="Update bug">
            <HelpTooltip text={bugUpdatesHelpText} />
          </GridLabel>
          <Grid container item xs={8} lg={2} alignItems="center">
            {mutateRule.isLoading && (<CircularProgress size="1rem" />)}
            <Switch
              data-testid="update-bug-toggle"
              aria-label="update bug"
              checked={rule.isManagingBug}
              onChange={handleToggleUpdateBug}
              disabled={mutateRule.isLoading}/>
          </Grid>
        </Grid>
        {
          // Only display bug prioity update toggle if bug update was enabled.
          rule.isManagingBug && (
            <Grid container item xs={12} justifyContent="flex-end">
              <GridLabel xs={4} lg={3} text="Update bug priority">
                <HelpTooltip text={bugPriorityUpdateHelpText} />
              </GridLabel>
              <Grid container item xs={8} lg={2} alignItems="center">
                {mutateRule.isLoading && (<CircularProgress size="1rem" />)}
                <Switch
                  data-testid="update-bug-priority-toggle"
                  aria-label="update bug priority"
                  checked={rule.isManagingBugPriority}
                  onChange={handleToggleUpdateBugPriority}
                  disabled={mutateRule.isLoading}/>
              </Grid>
            </Grid>
          )
        }

        <Box sx={{ py: 2 }}>
          <Divider />
        </Box>
        {
          isLoading && (
            <LinearProgress />
          )
        }
        {
          error && (
            <Container>
              <LoadErrorAlert
                entityName='bug details'
                error={error}/>
            </Container>
          )
        }
        {
          issue && (
            <Grid container rowGap={1}>
              <GridLabel xs={4} lg={2} text="Status" />
              <Grid container item xs={8} lg={10} data-testid="bug-status">
                <Chip label={issue.status.status} color={bugStatusColor(issue.status.status)} />
              </Grid>
              <GridLabel xs={4} lg={2} text="Summary" />
              <GridLabel xs={8} lg={10} testid="bug-summary" text={issue.summary} />
            </Grid>
          )
        }
      </Container>
      <BugEditDialog
        open={editDialogOpen}
        setOpen={setEditDialogOpen}/>
    </Paper>
  );
};

export default BugInfo;
