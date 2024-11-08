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

import CloseIcon from '@mui/icons-material/Close';
import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import IconButton from '@mui/material/IconButton';

import { Problem } from '@/clusters/tools/problems';

import { ProblemExplanationSection } from '../problem_explanation_section/problem_explanation_section';

interface Props {
  openProblem?: Problem;
  handleClose: () => void;
}

export const ProblemExplanationDialog = ({
  openProblem,
  handleClose,
}: Props) => {
  return (
    <Dialog
      open={openProblem !== undefined}
      onClose={handleClose}
      maxWidth="lg"
      fullWidth
    >
      <DialogTitle>
        {openProblem && <>Problem: {openProblem.policy.humanReadableName}</>}
        <IconButton
          aria-label="Close details of problems"
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
        {openProblem && <ProblemExplanationSection problem={openProblem} />}
      </DialogContent>
      <DialogActions>
        <Button
          data-testid="problem-explanation-dialog-close"
          onClick={handleClose}
        >
          Close
        </Button>
      </DialogActions>
    </Dialog>
  );
};
