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

import Alert from '@mui/material/Alert';
import Snackbar from '@mui/material/Snackbar';
import { useContext } from 'react';

import {
  SnackbarContext,
  snackContextDefaultState,
} from '@/clusters/context/snackbar_context';

const FeedbackSnackbar = () => {
  const { snack, setSnack } = useContext(SnackbarContext);

  const handleClose = () => {
    setSnack(snackContextDefaultState);
  };

  return (
    <Snackbar
      data-testid="snackbar"
      open={snack.open}
      autoHideDuration={6000}
      anchorOrigin={{ horizontal: 'center', vertical: 'bottom' }}
      onClose={handleClose}
    >
      <Alert
        onClose={handleClose}
        severity={snack.severity}
        sx={{ width: '100%' }}
      >
        {snack.message}
      </Alert>
    </Snackbar>
  );
};

export default FeedbackSnackbar;
