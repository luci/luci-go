// Copyright 2024 The LUCI Authors.
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

import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import Accordion from '@mui/material/Accordion';
import AccordionDetails from '@mui/material/AccordionDetails';
import AccordionSummary from '@mui/material/AccordionSummary';
import Chip from '@mui/material/Chip';
import Typography from '@mui/material/Typography';
import { useState } from 'react';

import { ExoneratedTestVariantBranch } from '@/clusters/hooks/use_fetch_exonerated_test_variant_branches';
import { TestStabilityCriteria } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variants.pb';

import {
  CriteriaMetIndicator,
  failureCriteriaMetIndicator,
  flakyCriteriaMetIndicator,
} from '../model/model';

import FailureCriteriaSection from './failure_criteria_section/failure_criteria_section';
import FlakyCriteriaSection from './flaky_criteria_section/flaky_criteria_section';

interface Props {
  criteria: TestStabilityCriteria;
  project: string;
  testVariantBranch: ExoneratedTestVariantBranch;
}

const ExonerationExplanationSection = ({
  criteria,
  testVariantBranch,
}: Props) => {
  const flakyCriteriaMet = flakyCriteriaMetIndicator(
    criteria,
    testVariantBranch,
  );
  const failureCriteriaMet = failureCriteriaMetIndicator(
    criteria,
    testVariantBranch,
  );

  let defaultExpanded = '';
  if (flakyCriteriaMet !== CriteriaMetIndicator.NotMet) {
    defaultExpanded = 'flaky';
  } else if (failureCriteriaMet !== CriteriaMetIndicator.NotMet) {
    defaultExpanded = 'failure';
  }

  const [expanded, setExpanded] = useState(defaultExpanded);

  return (
    <>
      <Accordion
        expanded={expanded === 'flaky'}
        onChange={(_, isExpanded: boolean) =>
          setExpanded(isExpanded ? 'flaky' : '')
        }
      >
        <AccordionSummary
          expandIcon={<ExpandMoreIcon />}
          data-testid="flaky_criteria_header"
        >
          <Typography variant="h5">
            Criteria: Run-flaky source verdicts&nbsp;
            <Chip
              color={criteriaMetColor(flakyCriteriaMet)}
              label={criteriaMetLabel(flakyCriteriaMet)}
              data-testid="flaky_criteria_met_chip"
            />
          </Typography>
        </AccordionSummary>
        <AccordionDetails data-testid="flaky_criteria_details">
          <FlakyCriteriaSection
            testVariantBranch={testVariantBranch}
            criteria={criteria}
          />
        </AccordionDetails>
      </Accordion>
      <Accordion
        expanded={expanded === 'failure'}
        onChange={(_, isExpanded: boolean) =>
          setExpanded(isExpanded ? 'failure' : '')
        }
      >
        <AccordionSummary
          expandIcon={<ExpandMoreIcon />}
          data-testid="failure_criteria_header"
        >
          <Typography variant="h5">
            Criteria: Recent test runs with failing results&nbsp;
            <Chip
              color={criteriaMetColor(failureCriteriaMet)}
              label={criteriaMetLabel(failureCriteriaMet)}
              data-testid="failure_criteria_met_chip"
            />
          </Typography>
        </AccordionSummary>
        <AccordionDetails data-testid="failure_criteria_details">
          <FailureCriteriaSection
            testVariantBranch={testVariantBranch}
            criteria={criteria}
          />
        </AccordionDetails>
      </Accordion>
    </>
  );
};

const criteriaMetLabel = (criteriaMet: CriteriaMetIndicator): string => {
  if (criteriaMet === CriteriaMetIndicator.Met) {
    return 'Met';
  } else if (criteriaMet === CriteriaMetIndicator.AlmostMet) {
    return 'Almost met';
  } else {
    return 'Not met';
  }
};

const criteriaMetColor = (criteriaMet: CriteriaMetIndicator) => {
  if (criteriaMet === CriteriaMetIndicator.Met) {
    return 'success';
  } else if (criteriaMet === CriteriaMetIndicator.AlmostMet) {
    return 'warning';
  } else {
    return 'default';
  }
};

export default ExonerationExplanationSection;
