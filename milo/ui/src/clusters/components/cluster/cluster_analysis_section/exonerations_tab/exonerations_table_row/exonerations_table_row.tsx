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

import CloseIcon from '@mui/icons-material/Close';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import IconButton from '@mui/material/IconButton';
import Link from '@mui/material/Link';
import TableCell, { tableCellClasses } from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
import { styled } from '@mui/material/styles';
import { DateTime } from 'luxon';
import { useState } from 'react';

import {
  ExoneratedTestVariant,
  ExonerationCriteria,
  isFailureCriteriaAlmostMet,
  isFailureCriteriaMet,
  isFlakyCriteriaAlmostMet,
  isFlakyCriteriaMet,
} from '@/clusters/components/cluster/cluster_analysis_section/exonerations_tab/model/model';
import { testHistoryLink } from '@/clusters/tools/urlHandling/links';
import { variantAsPairs } from '@/clusters/tools/variant_tools';
import { RelativeTimestamp } from '@/common/components/relative_timestamp';

import ExonerationExplanationSection from '../exoneration_explanation_section/exoneration_explanation_section';

const WrappingTableCell = styled(TableCell)(() => ({
  [`&.${tableCellClasses.root}`]: {
    overflowWrap: 'anywhere',
  },
}));

interface Props {
  criteria: ExonerationCriteria;
  project: string;
  testVariant: ExoneratedTestVariant;
}

const ExonerationsTableRow = ({ criteria, project, testVariant }: Props) => {
  const [open, setOpen] = useState(false);

  const handleClickOpen = () => {
    setOpen(true);
  };
  const handleClose = () => {
    setOpen(false);
  };
  const dialogTitle = (): string => {
    let variation: string;
    if (
      isFlakyCriteriaMet(criteria, testVariant) ||
      isFailureCriteriaMet(criteria, testVariant)
    ) {
      variation = '';
    } else if (
      isFlakyCriteriaAlmostMet(criteria, testVariant) ||
      isFailureCriteriaAlmostMet(criteria, testVariant)
    ) {
      variation = ' close to';
    } else {
      variation = ' not';
    }
    return `Why is this test variant${variation} being exonerated?`;
  };
  const statusLabel = (): string => {
    if (
      isFlakyCriteriaMet(criteria, testVariant) ||
      isFailureCriteriaMet(criteria, testVariant)
    ) {
      return 'Yes ';
    } else if (
      isFlakyCriteriaAlmostMet(criteria, testVariant) ||
      isFailureCriteriaAlmostMet(criteria, testVariant)
    ) {
      return 'No, but close to ';
    }
    return 'No ';
  };

  return (
    <TableRow>
      <WrappingTableCell data-testid="exonerations_table_test_cell">
        {testVariant.testId}
      </WrappingTableCell>
      <WrappingTableCell>
        {variantAsPairs(testVariant.variant)
          .map((vp) => vp.key + ': ' + vp.value)
          .join(', ')}
      </WrappingTableCell>
      <WrappingTableCell>
        <Link
          sx={{ display: 'inline-flex' }}
          aria-label="Test history link"
          href={testHistoryLink(
            project,
            testVariant.testId,
            testVariant.variant,
          )}
          target="_blank"
        >
          View
        </Link>
      </WrappingTableCell>
      <WrappingTableCell>{statusLabel()}</WrappingTableCell>
      <WrappingTableCell>
        <Chip
          variant="outlined"
          color="default"
          onClick={handleClickOpen}
          label={<Typography variant="button">more info</Typography>}
          sx={{ borderRadius: 1, float: 'right' }}
        />
        <Dialog open={open} onClose={handleClose} maxWidth="lg" fullWidth>
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
              }}
            >
              <CloseIcon />
            </IconButton>
          </DialogTitle>
          <DialogContent>
            <ExonerationExplanationSection
              criteria={criteria}
              project={project}
              testVariant={testVariant}
            />
          </DialogContent>
          <DialogActions>
            <Button onClick={handleClose}>Close</Button>
          </DialogActions>
        </Dialog>
      </WrappingTableCell>
      <WrappingTableCell data-testid="exonerations_table_critical_failures_cell">
        {testVariant.criticalFailuresExonerated}
      </WrappingTableCell>
      <WrappingTableCell>
        <RelativeTimestamp
          timestamp={DateTime.fromISO(testVariant.lastExoneration)}
        />
      </WrappingTableCell>
    </TableRow>
  );
};

export default ExonerationsTableRow;
