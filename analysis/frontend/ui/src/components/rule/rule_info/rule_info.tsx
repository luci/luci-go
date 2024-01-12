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
import { Link as RouterLink } from 'react-router-dom';

import Archive from '@mui/icons-material/Archive';
import Unarchive from '@mui/icons-material/Unarchive';
import LoadingButton from '@mui/lab/LoadingButton';
import Box from '@mui/material/Box';
import Container from '@mui/material/Container';
import Grid from '@mui/material/Grid';
import Link from '@mui/material/Link';
import Paper from '@mui/material/Paper';
import PanelHeading from '@/components/headings/panel_heading/panel_heading';

import ConfirmDialog from '@/components/confirm_dialog/confirm_dialog';
import GridLabel from '@/components/grid_label/grid_label';
import HelpTooltip from '@/components/help_tooltip/help_tooltip';
import RuleEditDialog from '@/components/rule/rule_edit_dialog/rule_edit_dialog';
import { useMutateRule } from '@/hooks/use_mutate_rule';
import { Rule, UpdateRuleRequest } from '@/proto/go.chromium.org/luci/analysis/proto/v1/rules.pb';
import { linkToCluster } from '@/tools/urlHandling/links';
import RuleDefinition from '../rule_definition/rule_definition';

const definitionTooltipText = 'The failures matched by this rule.';
const archivedTooltipText = 'Archived failure association rules do not match failures. If a rule is no longer needed, it should be archived.';
const sourceClusterTooltipText = 'The cluster this rule was originally created from.';

interface Props {
    project: string;
    rule: Rule;
}

const RuleInfo = ({ project, rule }: Props) => {
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [confirmDialogOpen, setConfirmDialogOpen] = useState(false);

  const mutateRule = useMutateRule();

  const toggleArchived = () => {
    const request: UpdateRuleRequest = UpdateRuleRequest.create({
      rule: {
        name: rule.name,
        isActive: !rule.isActive,
      },
      updateMask: Object.freeze(['isActive']),
      etag: rule.etag,
    });
    mutateRule.mutate(request);
  };

  const onArchiveConfirm = () => {
    setConfirmDialogOpen(false);
    toggleArchived();
  };

  const onArchiveCancel = () => {
    setConfirmDialogOpen(false);
  };

  return (
    <Paper data-cy="rule-info" elevation={3} sx={{ pt: 2, pb: 2, mt: 1 }} >
      <Container maxWidth={false}>
        <PanelHeading>
          Rule Details
        </PanelHeading>
        <Grid container rowGap={1}>
          <GridLabel text="Rule definition">
            <HelpTooltip text={definitionTooltipText} />
          </GridLabel>
          <Grid item xs={10} alignItems="center">
            <RuleDefinition definition={rule.ruleDefinition} onEditClicked={() => setEditDialogOpen(true)} />
          </Grid>
          <GridLabel text="Source cluster">
            <HelpTooltip text={sourceClusterTooltipText} />
          </GridLabel>
          <Grid item xs={10} alignItems="center">
            <Box sx={{ display: 'inline-block' }} paddingTop={1}>
              {
                rule.sourceCluster?.algorithm && rule.sourceCluster?.id ? (
                  <Link aria-label='source cluster link' component={RouterLink} to={linkToCluster(project, rule.sourceCluster)}>
                    {rule.sourceCluster.algorithm}/{rule.sourceCluster.id}
                  </Link>
                ) : (
                    'None'
                )
              }
            </Box>
          </Grid>
          <GridLabel text="Archived">
            <HelpTooltip text={archivedTooltipText} />
          </GridLabel>
          <Grid item xs={10} alignItems="center" columnGap={1}>
            <Box data-testid="rule-archived" sx={{ display: 'inline-block' }} paddingTop={1} paddingRight={1}>
              {rule.isActive ? 'No' : 'Yes'}
            </Box>
            <LoadingButton
              data-testid="rule-archived-toggle"
              loading={mutateRule.isLoading}
              variant="outlined"
              startIcon={rule.isActive ? (<Archive />) : (<Unarchive />)}
              onClick={() => setConfirmDialogOpen(true)}>
              {rule.isActive ? 'Archive' : 'Restore'}
            </LoadingButton>
          </Grid>
        </Grid>
      </Container>
      <ConfirmDialog
        open={confirmDialogOpen}
        message={
          rule.isActive?
          'Impact and recent failures are not available for archived rules.'+
          ' Automatic bug priority updates and auto-closure will also cease.'+
          ' You can restore archived rules at any time.' :
          'LUCI Analysis automatically archives rules when the associated'+
          ' bug has been closed for 30 days. Please make sure the associated'+
          ' bug is no longer closed to avoid this rule being automatically'+
          ' re-archived.'
        }
        onConfirm={onArchiveConfirm}
        onCancel={onArchiveCancel}/>
      <RuleEditDialog
        open={editDialogOpen}
        setOpen={setEditDialogOpen}
        rule={rule}/>
    </Paper>
  );
};

export default RuleInfo;
