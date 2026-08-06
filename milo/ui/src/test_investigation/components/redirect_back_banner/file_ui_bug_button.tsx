// Copyright 2026 The LUCI Authors.
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

import { Feedback as FeedbackIcon } from '@mui/icons-material';
import { Button, Link } from '@mui/material';

import { useAuthState } from '@/common/components/auth_state_provider';
import { genFeedbackUrl } from '@/common/tools/utils';

export function FileUIBugButton() {
  const { email } = useAuthState();
  const isGoogler = email?.endsWith('@google.com') ?? false;
  const feedbackBugtemplateComment = `You can use this entry to log an issue or provide a recommendation for the new Test Results Page.
Please include a short description of the issue or suggestion and, if applicable, describe steps to reproduce and attach a screenshot.
From Link: ${self.location.href}`;

  return (
    <Button
      component={Link}
      target="_blank"
      href={genFeedbackUrl({
        bugComponent: isGoogler ? '1838234' : '1456503',
        customComment: feedbackBugtemplateComment,
      })}
      color="primary"
      size="small"
      variant="contained"
      startIcon={<FeedbackIcon />}
      sx={{ textTransform: 'none' }}
    >
      File UI bug
    </Button>
  );
}
