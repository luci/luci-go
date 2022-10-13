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

import { useLocation, useNavigate } from 'react-router-dom';
import { Link as RouterLink } from 'react-router-dom';

import { styled } from '@mui/material/styles';
import Accordion from '@mui/material/Accordion';
import MuiAccordionSummary, {
  AccordionSummaryProps,
} from '@mui/material/AccordionSummary';
import AccordionDetails from '@mui/material/AccordionDetails';
import Container from '@mui/material/Container';
import EditIcon from '@mui/icons-material/Edit';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import FeedbackIcon from '@mui/icons-material/Feedback';
import Link from '@mui/material/Link';
import Typography, {
  TypographyProps,
} from '@mui/material/Typography';

import recentFailuresUrl from './recent_failures.png';
import ruleIdentifiersUrl from './failure_association_rule_identifiers.png';

const AccordionSummary = (props: AccordionSummaryProps) => (
  <MuiAccordionSummary
    expandIcon={<ExpandMoreIcon />}
    {...props} />
);

const AccordionHeading = styled(Typography)<TypographyProps>(() => ({
  'font-size': 18,
}));

const HelpPage = () => {
  const location = useLocation();
  const navigate = useNavigate();

  const handleChange =
    (panel: string) => (_: React.SyntheticEvent, isExpanded: boolean) => {
      if (isExpanded) {
        navigate({ hash: panel });
      } else {
        navigate({ hash: '' });
      }
    };

  return (
    <Container maxWidth="xl">
      <h1>Help</h1>
      <Accordion expanded={location.hash === '#rules'} onChange={handleChange('rules')}>
        <AccordionSummary>
          <AccordionHeading>
            How do I create or modify a failure association rule?
          </AccordionHeading>
        </AccordionSummary>
        <AccordionDetails>
          <Typography paragraph>
            Failure association rules define an association between a bug and a set of failures.
            <ul>
              <li><strong>To create a rule</strong>, use the <em>new rule</em> button on the rules page.</li>
              <li><strong>To modify a rule</strong>, from the page of that rule, click the edit button (<EditIcon />) next to the rule definition.</li>
            </ul>
          </Typography>
          <Typography paragraph>
            Failure association rules support a subset of&nbsp;
            <Link href="https://cloud.google.com/bigquery/docs/reference/standard-sql/query-syntax">Google Standard SQL syntax</Link> for
            boolean expressions, with the following limitations:
            <ul>
              <li>The only valid operators are <em>AND</em>, <em>OR</em>, <em>NOT</em>, =, &lt;&gt;, LIKE and parantheses.</li>
              <li>The only valid function is <Link href="https://cloud.google.com/bigquery/docs/reference/standard-sql/functions-and-operators#regexp_contains">REGEXP_CONTAINS</Link>.</li>
              <li>The only valid identifiers are <em>test</em> and <em>reason</em>.</li>
              <li>Strings must use double quotes.</li>
            </ul>
            Example: <em>reason LIKE &quot;Check failed: Expected foo, got %&quot; AND test LIKE &quot;MySuite%&quot;</em>.
          </Typography>
          <Typography paragraph>
            The information queriable via the <em>test</em> and <em>reason</em> identifiers is shown below. (The
            diagram shows a ci.chromium.org build page).
            <img src={ruleIdentifiersUrl} alt="Test is the fully-qualified test ID. It appears under the MILO Test Results tab
                    under the heading ID. The reason is the failure reason, which appears in the MILO Test Results tab under the heading Failure Reason."></img>
          </Typography>
          <Typography paragraph>
            If you do not see the failure reason section shown in the diagram, it means the test harness did not upload a&nbsp;
            <Link href="https://source.chromium.org/chromium/infra/infra/+/main:go/src/go.chromium.org/luci/resultdb/proto/v1/test_result.proto?q=FailureReason">failure reason</Link>.
            If you are interested in making a test harness upload failure reasons, please reach out using the send feedback (<FeedbackIcon />) button.
          </Typography>
          <Typography paragraph>
            We recommend associating failures with bugs using <em>failure reasons</em>, if possible. This is for two reasons:
            <ul>
              <li>
                Associating failures to bugs using <em>tests</em> usually leads to bugs that are more difficult to manage,
                as the tests can fail for multiple different reasons, and tracking the investigation of multiple
                reasons in one bug is hard.
              </li>
              <li>
                Multiple tests may be failing with the same failure reason, and maintaining one canoncial bug
                for that reason can avoid duplicating investigation work.
              </li>
            </ul>
          </Typography>
        </AccordionDetails>
      </Accordion>

      <Accordion expanded={location.hash === '#archive-rule'} onChange={handleChange('archive-rule')}>
        <AccordionSummary>
          <AccordionHeading>
            I no longer need a failure association rule, can I get rid of it?
          </AccordionHeading>
        </AccordionSummary>
        <AccordionDetails>
          <Typography paragraph>
            Yes! If you no longer need a rule, click the <em>Archive</em> button on the rule page.
          </Typography>
        </AccordionDetails>
      </Accordion>

      <Accordion expanded={location.hash === '#new-bug-filed'} onChange={handleChange('new-bug-filed')}>
        <AccordionSummary>
          <AccordionHeading>
            LUCI Analysis filed a bug. What should I do?
          </AccordionHeading>
        </AccordionSummary>
        <AccordionDetails>
          <Typography paragraph>
            LUCI Analysis files bugs for clusters when their impact exceeds the&nbsp;
            <Link href="https://source.chromium.org/chromium/infra/infra/+/main:go/src/go.chromium.org/luci/analysis/proto/config/project_config.proto?q=bug_filing_threshold">
              threshold configured by the project
            </Link>.
            When you receive a LUCI Analysis bug, the objective should be to reduce the impact by fixing the problematic test(s).
          </Typography>
          <Typography paragraph>
            <strong>Key point:</strong> the bug is for <em>impact</em>, not <em>failures</em>. To resolve the impact,
            you may not need to fix all failures. For example, failures occuring on experimental builders or tests
            with no impact would be irrelevant.
          </Typography>
          <Typography paragraph>
            To investigate a bug:
            <ul>
              <li>Navigate to the rule page for the bug (this link will be in the first comment on the bug after the issue description).</li>
              <li>Review the <em>Impact</em> panel to verify the impact is still occuring.</li>
              <li>
                Use the <em>Recent failures</em> panel to identify the tests which have impactful failures. Note that only some test variants
                (ways of running the test) may be broken. For example, the failure may only be happening on one device or operating system.
                You can sort the table and use the <em>group by</em> feature to help you.
              </li>
              <li>
                Review the history of the problematic test variants using the <em>History</em> link. Alternatively, look at examples by
                expanding the node for the test and clicking on the build- link in each row.
                <img src={recentFailuresUrl} alt="The recent failures panel has links to test history and failure examples."></img>
              </li>
            </ul>
          </Typography>
          <Typography paragraph>
            When investigating a set of test failures, you may identify that the bug is too broad (encompasses multiple different issues)
            or too narrow (only captures one part of a larger issue). If this applies, see <Link component={RouterLink} to='#combining-issues'>combining issues</Link>&nbsp;
            or <Link component={RouterLink} to='#splitting-issues'>splitting issues</Link>.
          </Typography>
        </AccordionDetails>
      </Accordion>
      <Accordion expanded={location.hash === '#priority-update'} onChange={handleChange('priority-update')}>
        <AccordionSummary>
          <AccordionHeading>
            LUCI Analysis updated the priority of a bug. Why?
          </AccordionHeading>
        </AccordionSummary>
        <AccordionDetails>
          <Typography paragraph>
          LUCI Analysis automatically adjusts the bug priority according to the impact of the bug&apos;s failures.
          The bug priority thresholds are specified your project&apos;s&nbsp;
            <Link href="https://source.chromium.org/chromium/infra/infra/+/main:go/src/go.chromium.org/luci/analysis/proto/config/project_config.proto?q=priorities">LUCI Analysis configuration</Link>.
          </Typography>
          <Typography paragraph>
            <strong>If the priority update is erroneous,</strong> manually set the bug priority to the correct value.
            LUCI Analysis will respect the priority you have set and will not change it.
          </Typography>
        </AccordionDetails>
      </Accordion>

      <Accordion expanded={location.hash === '#bug-verified'} onChange={handleChange('bug-verified')}>
        <AccordionSummary>
          <AccordionHeading>
            LUCI Analysis automatically verified a bug. Why?
          </AccordionHeading>
        </AccordionSummary>
        <AccordionDetails>
          <Typography paragraph>
            LUCI Analysis automatically closes bugs once the impact of the remaining failures falls below a&nbsp;
            <Link href="https://source.chromium.org/chromium/infra/infra/+/main:go/src/go.chromium.org/luci/analysis/proto/config/project_config.proto?q=%22closure%20threshold%22">project-specific threshold</Link>.
          </Typography>
          <Typography paragraph>
            <strong>If the bug closure is erroneous,</strong> go to the rule page (a link to this page is in the comment accompanying
            the the closure), and turn the <em>Update bug</em> slider in the <em>Associated bug</em> panel to off.
            Then manually set the bug state you desire.
          </Typography>
        </AccordionDetails>
      </Accordion>

      <Accordion expanded={location.hash === '#bug-reopened'} onChange={handleChange('bug-reopened')}>
        <AccordionSummary>
          <AccordionHeading>
            LUCI Analysis automatically re-opened a bug. Why?
          </AccordionHeading>
        </AccordionSummary>
        <AccordionDetails>
          <Typography paragraph>
            LUCI Analysis automatically re-opens bugs if their impact exceeds the threshold for filing new bugs.
          </Typography>
          <Typography paragraph>
            <strong>If the bug re-opening is erroneous:</strong>
            <ul>
              <li>If the re-opening is erroneous because you have just fixed a bug, this may be because there is
                a delay between when a fix lands in the tree and when the impact in LUCI Analysis falls to zero.
                During this time, manually closing the issue as <em>Fixed (Verified)</em> may lead LUCI Analysis
                to re-open your issue. To avoid this, we recommend you only mark your bugs <em>Fixed</em> and
                let LUCI Analysis verify the fix (this could take up to 7 days).</li>
              <li>
                Alternatively, you can stop LUCI Analysis overriding the bug by turning off the <em>Update bug</em>
                &nbsp;slider on the rule page.
              </li>
            </ul>
          </Typography>
        </AccordionDetails>
      </Accordion>

      <Accordion expanded={location.hash === '#combining-issues'} onChange={handleChange('combining-issues')}>
        <AccordionSummary>
          <AccordionHeading>
            I have multiple bugs dealing with the same issue. Can I combine them?
          </AccordionHeading>
        </AccordionSummary>
        <AccordionDetails>
          <Typography paragraph>
            Yes! Just merge the bugs in the issue tracker, as you would normally. LUCI Analysis will automatically
            combine the failure association rules when the next bug filing pass runs (every 15 minutes or so).
          </Typography>
          <Typography paragraph>
            Alternatively, you can <Link component={RouterLink} to='#rules'>modify the failure association rule</Link> to
            reflect your intent.
          </Typography>
          <Typography paragraph>
            <strong>Take care:</strong> Just because some of the failures of one bug are the same as another,
            it does not mean all failures are. We generally recommend defining bugs
            based on <em>failure reason</em> of the failure (i.e. <em>reason LIKE &quot;%Check failed: Expected foo, got %&quot;</em>),
            if possible. See <Link component={RouterLink} to='#rules'>modify a failure association rule</Link>.
          </Typography>
        </AccordionDetails>
      </Accordion>

      <Accordion expanded={location.hash === '#splitting-issues'} onChange={handleChange('splitting-issues')}>
        <AccordionSummary>
          <AccordionHeading>
            My bug is actually dealing with multiple different issues. Can I split it?
          </AccordionHeading>
        </AccordionSummary>
        <AccordionDetails>
          <Typography paragraph>
            If the failures can be distinguished using the <em>failure reason</em> of the test or the
            identity of the test itself, you can. Refer to <Link component={RouterLink} to='#rules'>
            modify a failure association rule</Link>.
          </Typography>
          <Typography paragraph>
            If the failures <strong>cannot</strong> be distinguished based on the <em>failure reason</em>&nbsp;
            or <em>test</em>, it is not possible to represent this split in LUCI Analysis. In this case, we recommend
            using the bug linked from LUCI Analysis as the parent bug to track all failures of the test,
            and using seperate (child) bugs to track the investigation of individual issues.
          </Typography>
        </AccordionDetails>
      </Accordion>

      <Accordion expanded={location.hash === '#setup'} onChange={handleChange('setup')}>
        <AccordionSummary>
          <AccordionHeading>
            How do I setup LUCI Analysis for my project?
          </AccordionHeading>
        </AccordionSummary>
        <AccordionDetails>
          <Typography paragraph>
            Googlers can follow the instructions at <Link href="http://goto.google.com/luci-analysis">go/luci-analysis</Link>.
          </Typography>
        </AccordionDetails>
      </Accordion>

      <Accordion expanded={location.hash === '#feedback'} onChange={handleChange('feedback')}>
        <AccordionSummary>
          <AccordionHeading>
            I have a problem or feedback. How do I report it?
          </AccordionHeading>
        </AccordionSummary>
        <AccordionDetails>
          <Typography paragraph>
            First, check if your issue is addressed here. If not, please use the send feedback (<FeedbackIcon />) button at the top-right of this page to send a report.
          </Typography>
        </AccordionDetails>
      </Accordion>
    </Container>
  );
};

export default HelpPage;
