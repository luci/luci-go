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
import {
  useState,
  MouseEvent,
} from 'react';

import Button from '@mui/material/Button';
import CloseIcon from '@mui/icons-material/Close';
import Dialog from '@mui/material/Dialog';
import DialogTitle from '@mui/material/DialogTitle';
import DialogContent from '@mui/material/DialogContent';
import DialogActions from '@mui/material/DialogActions';
import IconButton from '@mui/material/IconButton';
import Link from '@mui/material/Link';
import TableCell from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';

import {
  ExoneratedTestVariant,
  ExonerationCriteria,
  isFailureCriteriaAlmostMet,
  isFailureCriteriaMet,
  isFlakyCriteriaAlmostMet,
  isFlakyCriteriaMet,
} from '@/components/cluster/cluster_analysis_section/exonerations_tab/model/model';
import {
  testHistoryLink,
} from '@/tools/urlHandling/links';
import {
  variantAsPairs,
} from '@/services/shared_models';
import ExonerationExplanationSection from '../exoneration_explanation_section/exoneration_explanation_section';

interface Props {
  criteria: ExonerationCriteria;
  project: string;
  testVariant: ExoneratedTestVariant;
}

const ExonerationsTableRow = ({
  criteria,
  project,
  testVariant,
}: Props) => {
  const [open, setOpen] = useState(false);

  const handleClickOpen = (e: MouseEvent<HTMLAnchorElement>) => {
    setOpen(true);
    e.preventDefault();
  };
  const handleClose = () => {
    setOpen(false);
  };
  const dialogTitle = (): string => {
    let variation: string;
    if (isFlakyCriteriaMet(criteria, testVariant) || isFailureCriteriaMet(criteria, testVariant)) {
      variation = '';
    } else if (isFlakyCriteriaAlmostMet(criteria, testVariant) || isFailureCriteriaAlmostMet(criteria, testVariant)) {
      variation = ' close to';
    } else {
      variation = ' not';
    }
    return `Why is this test variant${variation} being exonerated?`;
  };

  return (
    <TableRow>
      <TableCell data-testid='exonerations_table_test_cell'>
        {testVariant.testId}
      </TableCell>
      <TableCell>
        {variantAsPairs(testVariant.variant).map((vp) => vp.key + ': ' + vp.value).join(', ')}
      </TableCell>
      <TableCell>
        <Link
          sx={{ display: 'inline-flex' }}
          aria-label='Test history link'
          href={testHistoryLink(project, testVariant.testId, testVariant.variant)}
          target="_blank">
          View
        </Link>
      </TableCell>
      <TableCell>
        {(isFlakyCriteriaMet(criteria, testVariant) || isFailureCriteriaMet(criteria, testVariant)) ? 'Yes ' :
          ((isFlakyCriteriaAlmostMet(criteria, testVariant) || isFailureCriteriaAlmostMet(criteria, testVariant)) ? 'No, but close to ' :
            'No ')}
        <Link onClick={handleClickOpen} href='#'>Why?</Link>
        <Dialog open={open} onClose={handleClose} maxWidth='lg' fullWidth>
          <DialogTitle>
            {dialogTitle()}
            <IconButton
              aria-label="close"
              onClick={handleClose}
              sx={{
                position: 'absolute',
                right: 8,
                top: 8,
                color: (theme) => theme.palette.grey[500],
              }}>
              <CloseIcon />
            </IconButton>
          </DialogTitle>
          <DialogContent>
            <ExonerationExplanationSection criteria={criteria} project={project} testVariant={testVariant} />
          </DialogContent>
          <DialogActions>
            <Button onClick={handleClose}>
              Close
            </Button>
          </DialogActions>
        </Dialog>
      </TableCell>
      <TableCell data-testid='exonerations_table_critical_failures_cell'>
        {testVariant.criticalFailuresExonerated}
      </TableCell>
      <TableCell>
        {dayjs(testVariant.lastExoneration).fromNow()}
      </TableCell>
    </TableRow>
  );
};

export default ExonerationsTableRow;
