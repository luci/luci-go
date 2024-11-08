// Copyright 2023 The LUCI Authors.
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

import Container from '@mui/material/Container';
import Paper from '@mui/material/Paper';
import Typography from '@mui/material/Typography';

import PanelHeading from '@/clusters/components/headings/panel_heading/panel_heading';

const RuleArchivedMessage = () => {
  return (
    <Paper
      data-cy="analysis-section"
      elevation={3}
      sx={{ pt: 2, pb: 2, mt: 1, mb: 3 }}
    >
      <Container maxWidth={false}>
        <Typography component="div">
          <PanelHeading>This Rule is Archived</PanelHeading>
          <p>
            <strong>
              If you came here from a bug, please check if the bug has been
              duplicated into another bug and use that bug instead.
            </strong>
          </p>
          <p>As an archived rule:</p>
          <ul>
            <li>
              Failures are no longer associated with this rule, even if they
              match the rule definition
            </li>
            <li>
              This rule will not prevent new bugs from being filed by LUCI
              Analysis
            </li>
            <li>The bug associated with this rule will no longer be updated</li>
          </ul>
          <p>To unarchive the rule, please use the Restore button above.</p>
        </Typography>
      </Container>
    </Paper>
  );
};

export default RuleArchivedMessage;
