// Copyright 2025 The LUCI Authors.
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

import HomeIcon from '@mui/icons-material/Home';
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Tooltip,
  Typography,
} from '@mui/material';
import { lazy, Suspense, useEffect, useState } from 'react';
import { Link } from 'react-router';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { useAuthState } from '@/common/components/auth_state_provider';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { getLoginUrl } from '@/common/tools/url_utils';
import sadBass from '@/fleet/assets/pngs/sad_bass.png';
import { FLEET_CONSOLE_BASE_URL } from '@/fleet/constants/paths';
import { focusRingSx } from '@/fleet/theme/recipes';
import {
  fleetColors,
  fontFamilies,
  radii,
  statusTones,
} from '@/fleet/theme/tokens';
import { isTyping } from '@/fleet/utils/field_typing';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';

import { FleetHelmet } from '../layouts/fleet_helmet';

const TenSecondBassGame = lazy(() =>
  import('./not_found_game/ten_second_bass_game').then((m) => ({
    default: m.TenSecondBassGame,
  })),
);

export const PageNotFoundPage = () => {
  const auth = useAuthState();
  const [playingGame, setPlayingGame] = useState(false);

  const isAnonymous = !auth.identity || auth.identity === ANONYMOUS_IDENTITY;
  const target =
    typeof window !== 'undefined'
      ? window.location.pathname + window.location.search + window.location.hash
      : '';
  const redirectTarget = target && target !== '/' ? target : '/ui/fleet';

  // Press Space on the 404 page to launch the hidden 10-Second Bass easter egg
  useEffect(() => {
    if (playingGame) return;
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.code !== 'Space' || e.repeat) return;
      const el = e.target;
      if (
        el instanceof Element &&
        (isTyping(el) ||
          el.closest('a, button, [role="button"], input, textarea'))
      ) {
        return;
      }
      e.preventDefault();
      setPlayingGame(true);
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [playingGame]);

  return (
    <Box sx={{ width: '100%' }}>
      {isAnonymous && (
        <Box sx={{ px: 3, pt: 2.5 }}>
          <Alert
            severity="info"
            sx={{ alignItems: 'center' }}
            action={
              <Button
                href={getLoginUrl(redirectTarget)}
                variant="text"
                color="primary"
                size="small"
                sx={{ flexShrink: 0, ml: 2 }}
              >
                Sign in
              </Button>
            }
          >
            Some pages require sign-in or access permissions to view. Sign in to
            continue.
          </Alert>
        </Box>
      )}
      {playingGame ? (
        <Suspense
          fallback={
            <Box
              sx={{
                display: 'flex',
                justifyContent: 'center',
                alignItems: 'center',
                minHeight: 420,
              }}
            >
              <CircularProgress size={32} />
            </Box>
          }
        >
          <TenSecondBassGame onExit={() => setPlayingGame(false)} />
        </Suspense>
      ) : (
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            flexDirection: 'column',
            my: 6,
            px: 3,
            textAlign: 'center',
          }}
        >
          <Typography
            variant="h1"
            sx={{ fontSize: 50, letterSpacing: '10.5px' }}
          >
            404
          </Typography>
          <Tooltip
            title="Psst… click the bass or press Space"
            placement="right"
          >
            <Box
              component="button"
              type="button"
              aria-label="Play 10-Second Bass 404 speedrun game"
              onClick={() => setPlayingGame(true)}
              sx={{
                ...focusRingSx,
                background: 'transparent',
                border: 'none',
                padding: 1,
                cursor: 'pointer',
                borderRadius: radii.md,
                transition: 'transform 0.18s ease',
                '&:hover': {
                  transform: 'translateY(-6px) rotate(-3deg) scale(1.03)',
                },
              }}
            >
              <img
                alt="logo"
                id="luci-icon"
                src={sadBass}
                css={{
                  width: '100%',
                  maxWidth: 360,
                  display: 'block',
                }}
              />
            </Box>
          </Tooltip>
          <Typography variant="h1" sx={{ fontSize: 50 }}>
            Page not found
          </Typography>
          <Box sx={{ mt: 3 }}>
            <Button
              component={Link}
              to={FLEET_CONSOLE_BASE_URL}
              variant="outlined"
              startIcon={<HomeIcon />}
            >
              Back to Fleet Console
            </Button>
          </Box>
          <Typography
            variant="caption"
            sx={{
              mt: 2.5,
              fontFamily: fontFamilies.mono,
              color: fleetColors.textTertiary,
            }}
          >
            Route status:{' '}
            <Box
              component="span"
              sx={{
                fontWeight: 600,
                color: statusTones.error.fg,
                backgroundColor: statusTones.error.bg,
                px: 0.5,
                py: 0.125,
                borderRadius: radii.sm,
              }}
            >
              NEEDS_REPAIR
            </Box>{' '}
            · click the bass or press Space to repair
          </Typography>
        </Box>
      )}
    </Box>
  );
};

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-404">
      <FleetHelmet pageTitle="Page not found" />
      <RecoverableErrorBoundary
        // See the documentation for `<LoginPage />` for why we handle error
        // this way.
        key="fleet-page-not-found"
      >
        <PageNotFoundPage />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
