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

import dayjs from 'dayjs';
import { useQuery } from 'react-query';
import { Link as RouterLink } from 'react-router-dom';

import Box from '@mui/material/Box';
import LinearProgress from '@mui/material/LinearProgress';
import Link from '@mui/material/Link';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';

import LoadErrorAlert from '@/components/load_error_alert/load_error_alert';
import {
  getRulesService,
  ListRulesRequest,
} from '@/services/rules';
import { linkToRule } from '@/tools/urlHandling/links';
import { prpcRetrier } from '@/services/shared_models';

interface Props {
  project: string;
}

const RulesTable = ({ project } : Props ) => {
  const rulesService = getRulesService();
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
          return b.lastUpdateTime.localeCompare(a.lastUpdateTime);
        });
        return sortedRules;
      }, {
        retry: prpcRetrier,
      },
  );
  if (isLoading) {
    return <LinearProgress />;
  }

  if (error) {
    return <LoadErrorAlert
      entityName='rules'
      error={error}
    />;
  }

  return (
    <TableContainer component={Box}>
      <Table data-testid="impact-table" size="small" sx={{ overflowWrap: 'anywhere' }}>
        <TableHead>
          <TableRow>
            <TableCell>Rule Definition</TableCell>
            <TableCell sx={{ width: '180px' }}>Bug</TableCell>
            <TableCell sx={{ width: '100px' }}>Last Updated</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {
            rules && (
              rules.map((rule) => (
                <TableRow key={rule.ruleId}>
                  <TableCell>
                    <Link component={RouterLink} to={linkToRule(rule.project, rule.ruleId)} underline="hover">
                      {rule.ruleDefinition || 'Click to see example failures.'}
                    </Link>
                  </TableCell>
                  <TableCell><Link href={rule.bug.url} underline="hover">{rule.bug.linkText}</Link></TableCell>
                  <TableCell>{dayjs.utc(rule.lastUpdateTime).local().fromNow()}</TableCell>
                </TableRow>
              ))
            )
          }
        </TableBody>
      </Table>
    </TableContainer>
  );
};

export default RulesTable;
