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

import ScienceIcon from '@mui/icons-material/Science';
import Alert, { AlertProps } from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Link from '@mui/material/Link';

import { genFeedbackUrl } from '@/common/tools/utils';

export interface LabsWarningAlertProps extends AlertProps {}

export function LabsWarningAlert(props: LabsWarningAlertProps) {
  return (
    <Alert severity="info" icon={<ScienceIcon />} {...props}>
      <AlertTitle>Page under construction</AlertTitle>
      This page is experimental. Please provide{' '}
      <Link href={genFeedbackUrl()} target="_blank">
        feedback
      </Link>
      . URLs can be updated without backwards compatibility.
    </Alert>
  );
}
