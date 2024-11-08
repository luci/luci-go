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
import Link from '@mui/material/Link';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';
import { Link as RouterLink } from 'react-router-dom';

import { parseRuleName } from '@/clusters/tools/rules';
import { linkToRule } from '@/clusters/tools/urlHandling/links';

const ruleLink = (ruleName: string): string => {
  const ruleKey = parseRuleName(ruleName);
  return linkToRule(ruleKey.project, ruleKey.ruleId);
};

interface Props {
  bugSystem: string;
  bugId: string;
  rules: readonly string[];
}

const MultiRulesFound = ({ bugSystem, bugId, rules }: Props) => {
  return (
    <>
      <Grid item xs={12}>
        Multiple projects have rules matching the specified bug ({bugSystem}:
        {bugId})
      </Grid>
      <Grid item xs={12}>
        <Table>
          <TableBody>
            {rules.map((rule) => (
              <TableRow key={rule}>
                <TableCell>
                  <Link component={RouterLink} to={ruleLink(rule)}>
                    {parseRuleName(rule).project}
                  </Link>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Grid>
    </>
  );
};

export default MultiRulesFound;
