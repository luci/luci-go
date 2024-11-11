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

import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import EditIcon from '@mui/icons-material/Edit';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import FeedbackIcon from '@mui/icons-material/Feedback';
import HistoryIcon from '@mui/icons-material/History';
import Accordion from '@mui/material/Accordion';
import AccordionDetails from '@mui/material/AccordionDetails';
import MuiAccordionSummary, {
  AccordionSummaryProps,
} from '@mui/material/AccordionSummary';
import Container from '@mui/material/Container';
import Link from '@mui/material/Link';
import Typography, { TypographyProps } from '@mui/material/Typography';
import { styled } from '@mui/material/styles';
import { useLocation, useNavigate, Link as RouterLink } from 'react-router-dom';

import FeedbackSnackbar from '@/clusters/components/error_snackbar/feedback_snackbar';
import PageHeading from '@/clusters/components/headings/page_heading/page_heading';
import { SnackbarContextWrapper } from '@/clusters/context/snackbar_context';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';

import ruleIdentifiersImageUrl from './images/failure_association_rule_identifiers.png';
import monorailComponentImageUrl from './images/monorail_component.png';
import recentFailuresImageUrl from './images/recent_failures.png';

const AccordionSummary = (props: AccordionSummaryProps) => (
  <MuiAccordionSummary expandIcon={<ExpandMoreIcon />} {...props} />
);

const AccordionHeading = styled(Typography)<TypographyProps>(() => ({
  'font-size': 18,
}));

export const HelpPage = () => {
  const location = useLocation();
  const navigate = useNavigate();

  const handleChange =
    (panel: string) => (_: React.SyntheticEvent, isExpanded: boolean) => {
      if (isExpanded) {
        navigate({ hash: panel }, { replace: true });
      } else {
        navigate({ hash: '' }, { replace: true });
      }
    };

  return (
    <Container maxWidth="xl">
      <PageHeading>Help</PageHeading>
      <Accordion
        expanded={location.hash === '#rules'}
        onChange={handleChange('rules')}
      >
        <AccordionSummary>
          <AccordionHeading>
            How do I create or modify a failure association rule?
          </AccordionHeading>
        </AccordionSummary>
        <AccordionDetails>
          <Typography paragraph>
            Failure association rules define an association between a bug and a
            set of failures.
            <ul>
              <li>
                <strong>To create a rule</strong>, use the <em>new rule</em>{' '}
                button on the rules page.
              </li>
              <li>
                <strong>To modify a rule</strong>, from the page of that rule,
                click the edit button (
                <EditIcon sx={{ verticalAlign: 'middle' }} />) next to the rule
                definition.
              </li>
            </ul>
          </Typography>
          <Typography paragraph>
            Failure association rules support a subset of&nbsp;
            <Link href="https://cloud.google.com/bigquery/docs/reference/standard-sql/query-syntax">
              Google Standard SQL syntax
            </Link>{' '}
            for boolean expressions, with the following limitations:
            <ul>
              <li>
                The only valid operators are <em>AND</em>, <em>OR</em>,{' '}
                <em>NOT</em>, =, &lt;&gt;, LIKE and parantheses.
              </li>
              <li>
                The only valid function is{' '}
                <Link href="https://cloud.google.com/bigquery/docs/reference/standard-sql/functions-and-operators#regexp_contains">
                  REGEXP_CONTAINS
                </Link>
                .
              </li>
              <li>
                The only valid identifiers are <em>test</em> and <em>reason</em>
                .
              </li>
              <li>Strings must use double quotes.</li>
            </ul>
            Example:{' '}
            <em>
              reason LIKE &quot;Check failed: Expected foo, got %&quot; AND test
              LIKE &quot;MySuite%&quot;
            </em>
            .
          </Typography>
          <Typography paragraph>
            The information queriable via the <em>test</em> and <em>reason</em>{' '}
            identifiers is shown below. (The diagram shows a ci.chromium.org
            build page).
            <img
              src={ruleIdentifiersImageUrl}
              alt="Test is the fully-qualified test ID. It appears under the LUCI UI Test Results tab
                    under the heading ID. The reason is the failure reason, which appears in the
                     LUCI UI Test Results tab under the heading Failure Reason."
            ></img>
          </Typography>
          <Typography paragraph>
            If you do not see the failure reason section shown in the diagram,
            it means the test harness did not upload a&nbsp;
            <Link href="https://source.chromium.org/chromium/infra/infra/+/main:go/src/go.chromium.org/luci/resultdb/proto/v1/test_result.proto?q=FailureReason">
              failure reason
            </Link>
            . If you are interested in making a test harness upload failure
            reasons, please reach out using the send feedback (
            <FeedbackIcon sx={{ verticalAlign: 'middle' }} />) button and we can
            provide instructions.
          </Typography>
          <Typography paragraph>
            We recommend associating failures with bugs using{' '}
            <em>failure reasons</em> instead of <em>tests</em>, if possible.
            This is for two reasons:
            <ul>
              <li>
                Associating failures to bugs using <em>tests</em> usually leads
                to bugs that are more difficult to manage, as the tests can fail
                for multiple different reasons, and tracking the investigation
                of multiple reasons in one bug is hard.
              </li>
              <li>
                Multiple tests may be failing with the same failure reason, and
                maintaining one canoncial bug for that reason can avoid
                duplicating investigation work.
              </li>
            </ul>
          </Typography>
        </AccordionDetails>
      </Accordion>
      <Accordion
        expanded={location.hash === '#find-rule'}
        onChange={handleChange('find-rule')}
      >
        <AccordionSummary>
          <AccordionHeading>
            How do I find the failure association rule for a bug?
          </AccordionHeading>
        </AccordionSummary>
        <AccordionDetails>
          <Typography paragraph>
            For LUCI Analysis auto-filed bugs, the easiest way to get to the
            rule is by following the link to it in the issue description or
            first comment.
          </Typography>
          <Typography paragraph>
            For other bugs, a link to the rule (if it exists) should appear in a
            comment further down the bug.
          </Typography>
          <Typography paragraph>
            The following URL schemes can also be used:
            <ul>
              <li>
                <strong>Buganizer bugs:</strong>{' '}
                https://luci-analysis.appspot.com/b/<code>BUGANIZER_ID</code>,
                where BUGANIZER_ID is substituted for the buganizer bug ID, e.g.{' '}
                <code>1234</code>.
              </li>
              <li>
                <strong>Monorail:</strong> https://luci-analysis.appspot.com/b/
                <code>MONORAIL_PROJECT</code>/<code>MONORAIL_ID</code>, where
                MONORAIL_PROJECT is the monorail project (e.g.{' '}
                <code>chromium</code>) and MONORAIL_ID is the monorail bug ID,
                e.g. <code>5678</code>.
              </li>
            </ul>
          </Typography>
        </AccordionDetails>
      </Accordion>
      <Accordion
        expanded={location.hash === '#archive-rule'}
        onChange={handleChange('archive-rule')}
      >
        <AccordionSummary>
          <AccordionHeading>
            I no longer need a failure association rule, can I get rid of it?
          </AccordionHeading>
        </AccordionSummary>
        <AccordionDetails>
          <Typography paragraph>
            Yes! If you no longer need a rule, click the <em>Archive</em> button
            on the{' '}
            <Link component={RouterLink} to="#find-rule">
              rule page
            </Link>
            .
          </Typography>
        </AccordionDetails>
      </Accordion>

      <Accordion
        expanded={location.hash === '#new-bug-filed'}
        onChange={handleChange('new-bug-filed')}
      >
        <AccordionSummary>
          <AccordionHeading>
            LUCI Analysis filed a bug. What should I do?
          </AccordionHeading>
        </AccordionSummary>
        <AccordionDetails>
          <Typography paragraph>
            LUCI Analysis files bugs for <strong>problems</strong> identified by{' '}
            <Link href="http://goto.google.com/luci-analysis-setup#project-configuration">
              policies configured by your project
            </Link>
            &nbsp;(
            <Link href="https://source.chromium.org/chromium/infra/infra/+/main:go/src/go.chromium.org/luci/analysis/proto/config/project_config.proto?q=%22BugManagementPolicy%20policies%22">
              schema
            </Link>
            ). When you receive a LUCI Analysis bug, the objective should be to
            fix these problems. Problem(s) can always be resolved by fixing the
            problematic test(s), but there may be other ways too. Click the{' '}
            <em>more info</em> button next to the problem on the{' '}
            <Link component={RouterLink} to="#find-rule">
              rule page
            </Link>{' '}
            to view its suggested resolution steps and resolution criteria.
          </Typography>
          <Typography paragraph>
            <strong>Key point:</strong> the bug is for <strong>problems</strong>
            , not <strong>failures</strong>. To fix the problems, you may not
            need to fix all failures. For example, depending on the problem,
            failures occuring on experimental builders or tests having no impact
            would be irrelevant.
          </Typography>
          <Typography paragraph>
            Once all problems have been resolved and verified as such by LUCI
            Analysis, the bug will be{' '}
            <Link component={RouterLink} to="#bug-verified">
              automatically be marked as verified by LUCI Analysis
            </Link>
            .
          </Typography>
          <Typography paragraph>
            To investigate a bug:
            <ul>
              <li>
                Navigate to the{' '}
                <Link component={RouterLink} to="#find-rule">
                  rule page
                </Link>{' '}
                for the bug.
              </li>
              <li>
                Review the <em>Impact</em> panel to verify the impact is still
                occuring.
              </li>
              <li>
                Use the <em>Recent failures</em> tab to identify the tests which
                have impactful failures. Note that only some test variants (ways
                of running the test) may be broken. For example, the failure may
                only be happening on one device or operating system. You can
                sort the table and use the <em>group by</em> feature to help
                you.
              </li>
              <li>
                Review the history of the problematic test variants using the
                history (<HistoryIcon sx={{ verticalAlign: 'middle' }} />)
                button. Alternatively, look at examples by expanding the node
                for the test with the expand (
                <ChevronRightIcon sx={{ verticalAlign: 'middle' }} />) button
                and clicking on the build link in each row.
                <br />
                <img
                  src={recentFailuresImageUrl}
                  alt="The recent failures panel has links to test history and failure examples."
                ></img>
              </li>
            </ul>
          </Typography>
          <Typography paragraph>
            When investigating a set of test failures, you may identify that the
            bug is too broad (encompasses multiple different issues) or too
            narrow (only captures one part of a larger issue). If this applies,
            see{' '}
            <Link component={RouterLink} to="#combining-issues">
              combining issues
            </Link>
            &nbsp; or{' '}
            <Link component={RouterLink} to="#splitting-issues">
              splitting issues
            </Link>
            .
          </Typography>
        </AccordionDetails>
      </Accordion>
      <Accordion
        expanded={location.hash === '#priority-update'}
        onChange={handleChange('priority-update')}
      >
        <AccordionSummary>
          <AccordionHeading>
            LUCI Analysis updated the priority of a bug. Why?
          </AccordionHeading>
        </AccordionSummary>
        <AccordionDetails>
          <Typography paragraph>
            LUCI Analysis automatically adjusts the bug priority to match the
            priority of the problems identified by your project. Problems can be
            viewed on the{' '}
            <Link component={RouterLink} to="#find-rule">
              rule page
            </Link>
            , under the heading <em>Problems</em>.
          </Typography>
          <Typography paragraph>
            Problems and their associated priority are controlled by{' '}
            <Link href="http://goto.google.com/luci-analysis-setup#project-configuration">
              policies configured by your project
            </Link>
            &nbsp;(
            <Link href="https://source.chromium.org/chromium/infra/infra/+/main:go/src/go.chromium.org/luci/analysis/proto/config/project_config.proto?q=%22BugManagementPolicy%20policies%22">
              schema
            </Link>
            ).
          </Typography>
          <Typography paragraph>
            <strong>If the priority update is erroneous,</strong> manually set
            the bug priority to the correct value. LUCI Analysis will respect
            the priority you have set and will not change it.
          </Typography>
        </AccordionDetails>
      </Accordion>
      <Accordion
        expanded={location.hash === '#bug-verified'}
        onChange={handleChange('bug-verified')}
      >
        <AccordionSummary>
          <AccordionHeading>
            LUCI Analysis automatically verified a bug. Why?
          </AccordionHeading>
        </AccordionSummary>
        <AccordionDetails>
          <Typography paragraph>
            LUCI Analysis automatically verifies bugs once all problems
            identified by your project have been resolved. Problems can be
            viewed on the{' '}
            <Link component={RouterLink} to="#find-rule">
              rule page
            </Link>
            , under the heading <em>Problems</em>.
          </Typography>
          <Typography paragraph>
            Problems are identified according to{' '}
            <Link href="http://goto.google.com/luci-analysis-setup#project-configuration">
              policies configured by your project
            </Link>
            &nbsp;(
            <Link href="https://source.chromium.org/chromium/infra/infra/+/main:go/src/go.chromium.org/luci/analysis/proto/config/project_config.proto?q=%22BugManagementPolicy%20policies%22">
              schema
            </Link>
            ).
          </Typography>
          <Typography paragraph>
            <strong>If the bug closure is erroneous,</strong> go to the{' '}
            <Link component={RouterLink} to="#find-rule">
              rule page
            </Link>
            , and turn the <em>Update bug</em> switch in the{' '}
            <em>Associated bug</em> panel to off. Then manually set the bug
            state you desire.
          </Typography>
        </AccordionDetails>
      </Accordion>

      <Accordion
        expanded={location.hash === '#bug-reopened'}
        onChange={handleChange('bug-reopened')}
      >
        <AccordionSummary>
          <AccordionHeading>
            LUCI Analysis automatically re-opened a bug. Why?
          </AccordionHeading>
        </AccordionSummary>
        <AccordionDetails>
          <Typography paragraph>
            LUCI Analysis automatically re-opens bugs if their impact exceeds
            the threshold for filing new bugs.
          </Typography>
          <Typography paragraph>
            <strong>If the bug re-opening is erroneous:</strong>
            <ul>
              <li>
                If the re-opening is erroneous because you have just fixed a
                bug, this may be because there is a delay between when a fix
                lands in the tree and when the impact in LUCI Analysis falls to
                zero. During this time, manually closing the issue as{' '}
                <em>Fixed (Verified)</em> may lead LUCI Analysis to re-open your
                issue. To avoid this, we recommend you only mark your bugs{' '}
                <em>Fixed</em> and let LUCI Analysis verify the fix (this could
                take up to 7 days).
              </li>
              <li>
                Alternatively, you can stop LUCI Analysis overriding the bug by
                turning off the <em>Update bug</em>
                &nbsp;switch on the{' '}
                <Link component={RouterLink} to="#find-rule">
                  rule page
                </Link>
                .
              </li>
            </ul>
          </Typography>
        </AccordionDetails>
      </Accordion>

      <Accordion
        expanded={location.hash === '#combining-issues'}
        onChange={handleChange('combining-issues')}
      >
        <AccordionSummary>
          <AccordionHeading>
            I have multiple bugs dealing with the same issue. Can I combine
            them?
          </AccordionHeading>
        </AccordionSummary>
        <AccordionDetails>
          <Typography paragraph>
            Yes! Just merge the bugs in the issue tracker, as you would
            normally. LUCI Analysis will automatically combine the failure
            association rules when the next bug filing pass runs (every 15
            minutes or so).
          </Typography>
          <Typography paragraph>
            Alternatively, you can{' '}
            <Link component={RouterLink} to="#rules">
              modify the failure association rule
            </Link>{' '}
            to reflect your intent.
          </Typography>
          <Typography paragraph>
            <strong>Take care:</strong> Just because some of the failures of one
            bug are the same as another, it does not mean all failures are. We
            generally recommend defining bugs based on <em>failure reason</em>{' '}
            of the failure (i.e.{' '}
            <em>reason LIKE &quot;%Check failed: Expected foo, got %&quot;</em>
            ), if possible. See{' '}
            <Link component={RouterLink} to="#rules">
              modify a failure association rule
            </Link>
            .
          </Typography>
        </AccordionDetails>
      </Accordion>

      <Accordion
        expanded={location.hash === '#splitting-issues'}
        onChange={handleChange('splitting-issues')}
      >
        <AccordionSummary>
          <AccordionHeading>
            My bug is actually dealing with multiple different issues. Can I
            split it?
          </AccordionHeading>
        </AccordionSummary>
        <AccordionDetails>
          <Typography paragraph>
            If the failures can be distinguished using the{' '}
            <em>failure reason</em> of the test or the identity of the test
            itself, you can. Refer to{' '}
            <Link component={RouterLink} to="#rules">
              modify a failure association rule
            </Link>
            .
          </Typography>
          <Typography paragraph>
            If the failures <strong>cannot</strong> be distinguished based on
            the <em>failure reason</em>&nbsp; or <em>test</em>, it is not
            possible to represent this split in LUCI Analysis. In this case, we
            recommend using the bug linked from LUCI Analysis as the parent bug
            to track all failures of the test, and using seperate (child) bugs
            to track the investigation of individual issues.
          </Typography>
        </AccordionDetails>
      </Accordion>
      <Accordion
        expanded={location.hash === '#component-selection'}
        onChange={handleChange('component-selection')}
      >
        <AccordionSummary>
          <AccordionHeading>
            How does LUCI Analysis determine which component(s) to assign to an
            auto-filed bug?
          </AccordionHeading>
        </AccordionSummary>
        <AccordionDetails>
          <Typography paragraph>
            When LUCI Analysis auto-files a bug for a cluster, it assigns it to
            all components which contribute at least 30% of the cluster&apos;s
            test failures.
          </Typography>
          <Typography paragraph>
            In chromium projects, the component associated with a test failure
            is controlled by&nbsp;
            <Link
              href="https://chromium.googlesource.com/infra/infra/+/HEAD/go/src/infra/tools/dirmd/README.md"
              target="_blank"
            >
              DIR_METADATA
            </Link>
            &nbsp;files in the directory hierarchy of the test. The component
            associated with a specific test failure can be validated by viewing
            the failure in LUCI UI (ci.chromium.org), and looking for the
            monorail_component tag.
          </Typography>
          <img
            src={monorailComponentImageUrl}
            alt="Monorail component is the component that test matches, it appears in LUCI UI in the test variant tags."
          />
          <Typography paragraph>
            If a failure does not have this tag, it means the test result
            uploader is not providing component information for the test.
          </Typography>
          <Typography paragraph>
            In most cases, bugs being assigned to the wrong component can be
            fixed by updating the DIR_METADATA for the test.
          </Typography>
        </AccordionDetails>
      </Accordion>
      <Accordion
        expanded={location.hash === '#setup'}
        onChange={handleChange('setup')}
      >
        <AccordionSummary>
          <AccordionHeading>
            How do I setup LUCI Analysis for my project?
          </AccordionHeading>
        </AccordionSummary>
        <AccordionDetails>
          <Typography paragraph>
            Googlers can follow the instructions at{' '}
            <Link href="http://goto.google.com/luci-analysis">
              go/luci-analysis
            </Link>
            .
          </Typography>
        </AccordionDetails>
      </Accordion>

      <Accordion
        expanded={location.hash === '#feedback'}
        onChange={handleChange('feedback')}
      >
        <AccordionSummary>
          <AccordionHeading>
            I have a problem or feedback. How do I report it?
          </AccordionHeading>
        </AccordionSummary>
        <AccordionDetails>
          <Typography paragraph>
            First, check if your issue is addressed here. If not, please use the
            send feedback (<FeedbackIcon sx={{ verticalAlign: 'middle' }} />)
            button at the top-right of this page to send a report.
          </Typography>
        </AccordionDetails>
      </Accordion>
    </Container>
  );
};

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="help">
      <RecoverableErrorBoundary
        // See the documentation in `<LoginPage />` to learn why we handle error
        // this way.
        key="help"
      >
        <SnackbarContextWrapper>
          <HelpPage />
          <FeedbackSnackbar />
        </SnackbarContextWrapper>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
