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

import Box from '@mui/material/Box';
import LinearProgress from '@mui/material/LinearProgress';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import MenuItem from '@mui/material/MenuItem';
import Select, { SelectChangeEvent } from '@mui/material/Select';

import LoadErrorAlert from '@/components/load_error_alert/load_error_alert';
import {
  getRulesService,
  ListRulesRequest,
} from '@/legacy_services/rules';
import { prpcRetrier } from '@/legacy_services/shared_models';
import { useFetchProjectConfig } from '@/hooks/use_fetch_project_config';

import { useProblemFilterParam } from './hooks';
import RuleRow from './rule_row/rule_row';
import ProblemChip from './problem_chip/problem_chip';

interface Props {
  project: string;
}

const RulesTable = ({ project } : Props ) => {
  const rulesService = getRulesService();

  const {
    isLoading: isConfigLoading,
    data: projectConfig,
    error: configError,
  } = useFetchProjectConfig(project);

  const { isLoading, data: rules, error } = useQuery(['rules', project],
      async () => {
        const request: ListRulesRequest = {
          parent: `projects/${encodeURIComponent(project || '')}`,
        };

        const response = await rulesService.list(request);

        const rules = response.rules || [];
        const sortedRules = rules.sort((a, b)=> {
          // These are RFC 3339-formatted date/time strings.
          // Because they are all use the same timezone, and RFC 3339
          // date/times are specified from most significant to least
          // significant, any string sort that produces a lexicographical
          // ordering should also sort by time.
          return b.lastAuditableUpdateTime.localeCompare(a.lastAuditableUpdateTime);
        });
        return sortedRules;
      }, {
        retry: prpcRetrier,
      },
  );

  const policyIDs = projectConfig?.bugManagement.policies?.map((p) => p.id) || [];
  const [problemFilter, setProblemFilter] = useProblemFilterParam(policyIDs);

  if (error) {
    return <LoadErrorAlert
      entityName='rules'
      error={error}
    />;
  }
  if (configError) {
    return <LoadErrorAlert
      entityName='project config'
      error={configError}
    />;
  }
  if (isLoading || isConfigLoading) {
    return <LinearProgress />;
  }


  const handleProblemFilterChange = (event: SelectChangeEvent<string>) => {
    const value = event.target.value;
    setProblemFilter(value, true);
  };

  const colorIndexFunc = (policyID: string): number => {
    return policyIDs.indexOf(policyID);
  };

  const filteredRules = (rules || []).filter((r) => problemFilter == '' || r.bugManagementState.policyState?.some((ps) => ps.policyId == problemFilter && ps.lastActivationTime));

  return (
    <>
      <Box sx={{ paddingTop: '8px', paddingBottom: '12px' }}>
        <FormControl fullWidth data-testid="problem_filter">
          <InputLabel id="problem_filter_label">Problem filter</InputLabel>
          <Select
            labelId="problem_filter_label"
            id="problem_filter"
            label="Problem filter"
            value={problemFilter}
            onChange={handleProblemFilterChange}
            inputProps={{ 'data-testid': 'problem_filter_input' }}>
            <MenuItem value="">All rules</MenuItem>
            {projectConfig?.bugManagement.policies?.map((policy) => (
              <MenuItem
                key={policy.id}
                value={policy.id}>
                Only rules with problem&nbsp;
                <ProblemChip
                  key={policy.id}
                  policy={policy}
                  active
                  colorIndex={colorIndexFunc(policy.id)}
                />&nbsp;/&nbsp;{policy.humanReadableName}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
      </Box>
      <TableContainer component={Box}>
        <Table data-testid="impact-table" size="small" sx={{ overflowWrap: 'anywhere' }}>
          <TableHead>
            <TableRow>
              <TableCell>Rule Definition</TableCell>
              <TableCell>Problems</TableCell>
              <TableCell sx={{ width: '180px' }}>Bug</TableCell>
              <TableCell sx={{ width: '100px' }}>Last Updated</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {
              filteredRules.map((rule) => (
                <RuleRow
                  key={rule.ruleId}
                  rule={rule}
                  bugManagementConfig={projectConfig?.bugManagement}
                  focusPolicyID={problemFilter}
                  colorIndexFunc={colorIndexFunc} />
              ))
            }
          </TableBody>
        </Table>
      </TableContainer>
    </>
  );
};

export default RulesTable;
