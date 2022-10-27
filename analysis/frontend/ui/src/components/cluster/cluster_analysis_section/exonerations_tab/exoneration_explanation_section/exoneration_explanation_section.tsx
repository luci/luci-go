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

import Accordion from '@mui/material/Accordion';
import AccordionSummary from '@mui/material/AccordionSummary';
import AccordionDetails from '@mui/material/AccordionDetails';
import Chip from '@mui/material/Chip';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import Typography from '@mui/material/Typography';

import {
  isFailureCriteriaAlmostMet,
  isFailureCriteriaMet,
  isFlakyCriteriaAlmostMet,
  isFlakyCriteriaMet,
  ExoneratedTestVariant,
} from '../model/model';
import FlakyCriteriaSection from './flaky_criteria_section/flaky_criteria_section';
import FailureCriteriaSection from './failure_criteria_section/failure_criteria_section';

interface Props {
  project: string;
  testVariant: ExoneratedTestVariant;
}

const ExonerationExplanationSection = ({
  testVariant,
}: Props) => {
  const defaultExpanded = (): string => {
    if (isFlakyCriteriaMet(testVariant) || isFlakyCriteriaAlmostMet(testVariant)) {
      return 'flaky';
    } else if (isFailureCriteriaAlmostMet(testVariant) || isFailureCriteriaMet(testVariant)) {
      return 'failure';
    }
    return '';
  };
  const [expanded, setExpanded] = useState(defaultExpanded());

  const handleFlakyExpandedChange = (_: React.SyntheticEvent<Element, Event>, isExpanded: boolean) => {
    setExpanded(isExpanded ? 'flaky' : '');
  };
  const handleFailureExpandedChange = (_: React.SyntheticEvent<Element, Event>, isExpanded: boolean) => {
    setExpanded(isExpanded ? 'failure' : '');
  };
  const flakyCriteriaMetLabel = (): string => {
    if (isFlakyCriteriaMet(testVariant)) {
      return 'Met';
    } else if (isFlakyCriteriaAlmostMet(testVariant)) {
      return 'Almost met';
    } else {
      return 'Not met';
    }
  };
  const flakyCriteriaMetColor = () => {
    if (isFlakyCriteriaMet(testVariant)) {
      return 'success';
    } else if (isFlakyCriteriaAlmostMet(testVariant)) {
      return 'warning';
    } else {
      return 'default';
    }
  };
  const failureCriteriaMetLabel = (): string => {
    if (isFailureCriteriaMet(testVariant)) {
      return 'Met';
    } else if (isFailureCriteriaAlmostMet(testVariant)) {
      return 'Almost met';
    } else {
      return 'Not met';
    }
  };
  const failureCriteriaMetColor = () => {
    if (isFailureCriteriaMet(testVariant)) {
      return 'success';
    } else if (isFailureCriteriaAlmostMet(testVariant)) {
      return 'warning';
    } else {
      return 'default';
    }
  };

  return (
    <>
      <Accordion expanded={expanded == 'flaky'} onChange={handleFlakyExpandedChange}>
        <AccordionSummary
          expandIcon={<ExpandMoreIcon />}
          data-testid='flaky_criteria_header'
        >
          <Typography variant="h5">
            Criteria: Run-flaky verdicts&nbsp;
            <Chip
              color={flakyCriteriaMetColor()}
              label={flakyCriteriaMetLabel()}
              data-testid='flaky_criteria_met_chip'
            />
          </Typography>
        </AccordionSummary>
        <AccordionDetails data-testid='flaky_criteria_details'>
          <FlakyCriteriaSection testVariant={testVariant} />
        </AccordionDetails>
      </Accordion>
      <Accordion expanded={expanded == 'failure'} onChange={handleFailureExpandedChange}>
        <AccordionSummary
          expandIcon={<ExpandMoreIcon />}
          data-testid='failure_criteria_header'
        >
          <Typography variant="h5">
            Criteria: Recent verdicts with unexpected results&nbsp;
            <Chip
              color={failureCriteriaMetColor()}
              label={failureCriteriaMetLabel()}
              data-testid='failure_criteria_met_chip'
            />
          </Typography>
        </AccordionSummary>
        <AccordionDetails data-testid='failure_criteria_details'>
          <FailureCriteriaSection testVariant={testVariant} />
        </AccordionDetails>
      </Accordion>
    </>
  );
};

export default ExonerationExplanationSection;
