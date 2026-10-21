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

import ArticleOutlinedIcon from '@mui/icons-material/ArticleOutlined';
import FeedbackOutlinedIcon from '@mui/icons-material/FeedbackOutlined';
import LoginIcon from '@mui/icons-material/Login';
import { Box, Button, Card, Typography } from '@mui/material';
import { useLocation } from 'react-router';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { useAuthState } from '@/common/components/auth_state_provider';
import { getLoginUrl } from '@/common/tools/url_utils';
import { genFeedbackUrl } from '@/common/tools/utils';
import fleetConsoleMascot from '@/fleet/assets/pngs/fleet-console-mascot.png';
import { FEEDBACK_BUGANIZER_BUG_ID } from '@/fleet/constants/feedback';
import { fleetColors, fontFamilies } from '@/fleet/theme/tokens';

export const NotSignedInPage = () => {
  const { pathname, search, hash } = useLocation();
  const target = pathname + search + hash;
  const redirectTarget = target && target !== '/' ? target : '/ui/fleet';

  return (
    <Box
      data-testid="fleet-not-signed-in-page"
      sx={{
        px: { xs: 2, md: 4 },
        py: { xs: 6, md: 10 },
        maxWidth: 580,
        mx: 'auto',
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
      }}
    >
      <Card
        variant="surface"
        sx={{
          width: '100%',
          p: { xs: 3, sm: 5 },
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          textAlign: 'center',
        }}
      >
        <Box
          component="img"
          alt="Fleet Console mascot"
          src={fleetConsoleMascot}
          sx={{ width: 80, height: 80, mb: 2.5 }}
        />
        <Typography
          variant="h5"
          component="h1"
          sx={{ color: fleetColors.textPrimary, fontWeight: 500, mb: 1.5 }}
        >
          Sign in to Fleet Console
        </Typography>
        <Typography
          variant="body1"
          color="text.secondary"
          sx={{ mb: 3.5, maxWidth: 440 }}
        >
          Sign in to access fleet devices, lab health, and repair tooling.
        </Typography>
        <Button
          variant="contained"
          size="large"
          href={getLoginUrl(redirectTarget)}
          startIcon={<LoginIcon />}
          sx={{ px: 4, py: 1.25, fontSize: '1rem', fontWeight: 500 }}
        >
          Sign in
        </Button>
        <Typography
          variant="caption"
          color="text.secondary"
          sx={{ mt: 3, display: 'block' }}
        >
          After signing in, you will be redirected to:{' '}
          <Box
            component="span"
            sx={{
              fontFamily: fontFamilies.mono,
              fontWeight: 500,
              color: fleetColors.textPrimary,
              wordBreak: 'break-all',
            }}
          >
            {redirectTarget}
          </Box>
        </Typography>
      </Card>
      <Box
        sx={{
          mt: 4,
          display: 'flex',
          flexWrap: 'wrap',
          gap: 2,
          justifyContent: 'center',
        }}
      >
        <Button
          variant="toolbar"
          color="inherit"
          component="a"
          href="http://goto.google.com/fleet-console"
          target="_blank"
          rel="noreferrer"
          startIcon={<ArticleOutlinedIcon />}
        >
          Learn more
        </Button>
        <Button
          variant="toolbar"
          color="inherit"
          component="a"
          href={genFeedbackUrl({ bugComponent: FEEDBACK_BUGANIZER_BUG_ID })}
          target="_blank"
          rel="noreferrer"
          startIcon={<FeedbackOutlinedIcon />}
        >
          File feedback
        </Button>
      </Box>
    </Box>
  );
};

export const LoggedInBoundary = ({
  children,
}: {
  children: React.ReactNode;
}) => {
  const auth = useAuthState();
  if (!auth.identity || auth.identity === ANONYMOUS_IDENTITY) {
    return <NotSignedInPage />;
  }
  return children;
};
