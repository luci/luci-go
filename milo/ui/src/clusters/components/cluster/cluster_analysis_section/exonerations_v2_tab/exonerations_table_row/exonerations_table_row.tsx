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

import { ExoneratedTestVariantBranch } from '@/clusters/hooks/use_fetch_exonerated_test_variant_branches';
import {
  sourceRefLink,
  testHistoryLink,
} from '@/clusters/tools/urlHandling/links';
import { variantAsPairs } from '@/clusters/tools/variant_tools';
import { RelativeTimestamp } from '@/common/components/relative_timestamp';
import { displayApproxDuration } from '@/common/tools/time_utils';
import { TestStabilityCriteria } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variants.pb';

import ExonerationExplanationSection from '../exoneration_explanation_section/exoneration_explanation_section';
import { CriteriaMetIndicator, anyCriteriaMetIndicator } from '../model/model';

const WrappingTableCell = styled(TableCell)(() => ({
  [`&.${tableCellClasses.root}`]: {
    overflowWrap: 'anywhere',
  },
}));

interface Props {
  criteria: TestStabilityCriteria;
  project: string;
  testVariantBranch: ExoneratedTestVariantBranch;
}

const ExonerationsTableRow = ({
  criteria,
  project,
  testVariantBranch,
}: Props) => {
  const [open, setOpen] = useState(false);

  const handleClickOpen = () => {
    setOpen(true);
  };
  const handleClose = () => {
    setOpen(false);
  };

  const anyCriteriaMet = anyCriteriaMetIndicator(criteria, testVariantBranch);

  return (
    <TableRow>
      <WrappingTableCell data-testid="exonerations_table_test_cell">
        {testVariantBranch.testId}
      </WrappingTableCell>
      <WrappingTableCell>
        {variantAsPairs(testVariantBranch.variant)
          .map((vp) => vp.key + ': ' + vp.value)
          .join(', ')}
      </WrappingTableCell>
      <WrappingTableCell>
        <Link
          sx={{ display: 'inline-flex' }}
          href={sourceRefLink(testVariantBranch.sourceRef)}
          target="_blank"
        >
          {testVariantBranch.sourceRef.gitiles?.ref}
        </Link>
      </WrappingTableCell>
      <WrappingTableCell>
        <Link
          sx={{ display: 'inline-flex' }}
          aria-label="Test history link"
          href={testHistoryLink(
            project,
            testVariantBranch.testId,
            testVariantBranch.variant,
          )}
          target="_blank"
        >
          View
        </Link>
      </WrappingTableCell>
      <WrappingTableCell>
        <Chip
          color={statusColor(anyCriteriaMet)}
          label={statusLabel(anyCriteriaMet)}
        />
      </WrappingTableCell>
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
            {dialogTitle(anyCriteriaMet)}
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
              testVariantBranch={testVariantBranch}
            />
          </DialogContent>
          <DialogActions>
            <Button onClick={handleClose}>Close</Button>
          </DialogActions>
        </Dialog>
      </WrappingTableCell>
      <WrappingTableCell data-testid="exonerations_table_critical_failures_cell">
        {testVariantBranch.criticalFailuresExonerated}
      </WrappingTableCell>
      <WrappingTableCell>
        <RelativeTimestamp
          formatFn={displayApproxDuration}
          timestamp={DateTime.fromISO(testVariantBranch.lastExoneration)}
        />
      </WrappingTableCell>
    </TableRow>
  );
};

const dialogTitle = (anyCriteriaMet: CriteriaMetIndicator): string => {
  let variation: string;
  if (anyCriteriaMet === CriteriaMetIndicator.Met) {
    variation = '';
  } else if (anyCriteriaMet === CriteriaMetIndicator.AlmostMet) {
    variation = ' close to';
  } else {
    variation = ' not';
  }
  return `Why is this test variant${variation} being exonerated?`;
};

const statusColor = (anyCriteriaMet: CriteriaMetIndicator) => {
  if (anyCriteriaMet === CriteriaMetIndicator.Met) {
    return 'success';
  } else if (anyCriteriaMet === CriteriaMetIndicator.AlmostMet) {
    return 'warning';
  }
  return 'default';
};

const statusLabel = (anyCriteriaMet: CriteriaMetIndicator): string => {
  if (anyCriteriaMet === CriteriaMetIndicator.Met) {
    return 'Yes ';
  } else if (anyCriteriaMet === CriteriaMetIndicator.AlmostMet) {
    return 'No, but close to ';
  }
  return 'No ';
};

export default ExonerationsTableRow;
