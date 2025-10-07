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

import Edit from '@mui/icons-material/Edit';
import CircularProgress from '@mui/material/CircularProgress';
import Container from '@mui/material/Container';
import Grid from '@mui/material/Grid2';
import IconButton from '@mui/material/IconButton';
import Link from '@mui/material/Link';
import Paper from '@mui/material/Paper';
import Switch from '@mui/material/Switch';
import { useState } from 'react';

import GridLabel from '@/clusters/components/grid_label/grid_label';
import PanelHeading from '@/clusters/components/headings/panel_heading/panel_heading';
import HelpTooltip from '@/clusters/components/help_tooltip/help_tooltip';
import BugEditDialog from '@/clusters/components/rule/bug_edit_dialog/bug_edit_dialog';
import { useMutateRule } from '@/clusters/hooks/use_mutate_rule';
import {
  Rule,
  UpdateRuleRequest,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/rules.pb';

const bugUpdatesHelpText =
  'Whether the associated bug should be automatically verified (or re-opened)' +
  ' based on cluster impact. Only one rule may be set to' +
  ' update a given bug at any one time.';

const bugPriorityUpdateHelpText =
  'Whether the priority of the associated bug should be' +
  ' automatically updated based on cluster impact.';

interface Props {
  rule: Rule;
}

const BugInfo = ({ rule }: Props) => {
  const [editDialogOpen, setEditDialogOpen] = useState(false);

  // Rules always have a bug set.
  const bug = rule.bug!;

  const mutateRule = useMutateRule();

  const handleToggleUpdateBug = () => {
    const request: UpdateRuleRequest = UpdateRuleRequest.create({
      rule: {
        name: rule.name,
        isManagingBug: !rule.isManagingBug,
      },
      updateMask: Object.freeze(['isManagingBug']),
      etag: rule.etag,
    });
    mutateRule.mutate(request);
  };

  const handleToggleUpdateBugPriority = () => {
    const request: UpdateRuleRequest = UpdateRuleRequest.create({
      rule: {
        name: rule.name,
        isManagingBugPriority: !rule.isManagingBugPriority,
      },
      updateMask: Object.freeze(['isManagingBugPriority']),
      etag: rule.etag,
    });
    mutateRule.mutate(request);
  };

  return (
    <Paper data-cy="bug-info" elevation={3} sx={{ pt: 2, pb: 2, mt: 1 }}>
      <Container maxWidth={false}>
        <PanelHeading>Associated Bug</PanelHeading>
        <Grid container rowGap={0}>
          <GridLabel xs={4} lg={2} text="Bug"></GridLabel>
          <Grid
            container
            alignItems="center"
            columnGap={1}
            size={{
              xs: 8,
              lg: 5,
            }}
          >
            <Link data-testid="bug" target="_blank" href={bug.url}>
              {bug.linkText}
            </Link>
            <IconButton
              data-testid="bug-edit"
              aria-label="edit"
              onClick={() => setEditDialogOpen(true)}
            >
              <Edit />
            </IconButton>
          </Grid>
          <GridLabel xs={4} lg={3} text="Update bug">
            <HelpTooltip text={bugUpdatesHelpText} />
          </GridLabel>
          <Grid
            container
            alignItems="center"
            size={{
              xs: 8,
              lg: 2,
            }}
          >
            {mutateRule.isPending && <CircularProgress size="1rem" />}
            <Switch
              data-testid="update-bug-toggle"
              aria-label="update bug"
              checked={rule.isManagingBug}
              onChange={handleToggleUpdateBug}
              disabled={mutateRule.isPending}
            />
          </Grid>
        </Grid>
        {
          // Only display bug prioity update toggle if bug update was enabled.
          rule.isManagingBug && (
            <Grid container justifyContent="flex-end" size={12}>
              <GridLabel xs={4} lg={3} text="Update bug priority">
                <HelpTooltip text={bugPriorityUpdateHelpText} />
              </GridLabel>
              <Grid
                container
                alignItems="center"
                size={{
                  xs: 8,
                  lg: 2,
                }}
              >
                {mutateRule.isPending && <CircularProgress size="1rem" />}
                <Switch
                  data-testid="update-bug-priority-toggle"
                  aria-label="update bug priority"
                  checked={rule.isManagingBugPriority}
                  onChange={handleToggleUpdateBugPriority}
                  disabled={mutateRule.isPending}
                />
              </Grid>
            </Grid>
          )
        }
      </Container>
      <BugEditDialog open={editDialogOpen} setOpen={setEditDialogOpen} />
    </Paper>
  );
};

export default BugInfo;
