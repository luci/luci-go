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

import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import Typography from '@mui/material/Typography';

type HandleFunction = () => void;

interface Props {
  message?: string;
  open: boolean;
  onConfirm: HandleFunction;
  onCancel: HandleFunction;
}

const ConfirmDialog = ({ message = '', open, onConfirm, onCancel }: Props) => {
  return (
    <Dialog open={open} maxWidth="xs" fullWidth>
      <DialogTitle>Are you sure?</DialogTitle>
      {message && (
        <DialogContent>
          <Typography>{message}</Typography>
        </DialogContent>
      )}
      <DialogActions>
        <Button
          variant="outlined"
          onClick={onCancel}
          data-testid="confirm-dialog-cancel"
        >
          Cancel
        </Button>
        <Button
          variant="contained"
          onClick={onConfirm}
          data-testid="confirm-dialog-confirm"
        >
          Confirm
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default ConfirmDialog;
