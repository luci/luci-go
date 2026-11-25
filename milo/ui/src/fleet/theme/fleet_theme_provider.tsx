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

import { GlobalStyles, ThemeProvider, useMediaQuery } from '@mui/material';
import {
  ReactNode,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'react';

import { useFleetAnalytics } from '@/fleet/hooks/use_fleet_analytics';

import {
  FLEET_THEME_STORAGE_KEY,
  FleetThemeChoice,
  FleetThemeContext,
  FleetThemeToggleSource,
  isFleetThemeChoice,
} from './fleet_theme_context';
import {
  FLEET_PALETTES,
  FleetPalette,
  FleetThemeId,
  fleetThemeCssVars,
  luciCommonCssVars,
} from './palettes';
import { createFleetTheme, theme as lightTheme } from './theme';

const THEME_SESSION_TRACKED_KEY = 'fleet-console-theme-session-tracked';

function readStoredThemeChoice(): FleetThemeChoice | undefined {
  try {
    const raw = window.localStorage.getItem(FLEET_THEME_STORAGE_KEY);
    return isFleetThemeChoice(raw) ? raw : undefined;
  } catch {
    return undefined;
  }
}

function writeStoredThemeChoice(next: FleetThemeChoice): void {
  try {
    window.localStorage.setItem(FLEET_THEME_STORAGE_KEY, next);
  } catch {
    // Ignore storage quota or private-browsing errors; in-memory state still updates.
  }
}

function markSessionTracked(): boolean {
  try {
    if (window.sessionStorage.getItem(THEME_SESSION_TRACKED_KEY)) {
      return false;
    }
    window.sessionStorage.setItem(THEME_SESSION_TRACKED_KEY, '1');
    return true;
  } catch {
    return true;
  }
}

/**
 * Dark-mode overrides for the shared `CodeMirrorEditor` (which imports
 * light-only `codemirror.css` from `generic_libs/`). Kept inside
 * `FleetThemeProvider` so Fleet dark mode stays strictly scoped to `src/fleet/`.
 */
function codeMirrorDarkStyles(p: FleetPalette) {
  const c = p.colors;
  const t = p.tones;
  return {
    '.CodeMirror': {
      backgroundColor: c.surface,
      color: c.textPrimary,
    },
    '.CodeMirror-gutters': {
      backgroundColor: c.panelSubtle,
      borderRight: `1px solid ${c.divider}`,
    },
    '.CodeMirror-linenumber': {
      color: c.textTertiary,
    },
    '.CodeMirror-cursor': {
      borderLeftColor: c.textPrimary,
    },
    '.CodeMirror-selected': {
      backgroundColor: `${c.primaryContainer} !important`,
    },
    // `.CodeMirror-code` lifts specificity above CodeMirrorEditor's styled container.
    '.CodeMirror .CodeMirror-code .cm-property.cm-string': {
      color: t.teal.onContainer,
    },
    '.CodeMirror .CodeMirror-code .cm-string:not(.cm-property)': {
      color: t.success.onContainer,
    },
    '.CodeMirror .CodeMirror-code .cm-number': {
      color: t.caution.onContainer,
    },
    '.CodeMirror .CodeMirror-code .cm-atom, .CodeMirror .CodeMirror-code .cm-keyword':
      {
        color: t.special.onContainer,
      },
    '.CodeMirror .CodeMirror-code .cm-comment': {
      color: c.textTertiary,
    },
  };
}

/**
 * The one place that switches colors between light and dark. It provides the
 * MUI theme for the active palette and, for dark, writes the palette as CSS
 * custom properties on `:root` (so portaled menus, tooltips and dialogs follow
 * too). `fleetColors`, `statusTones`, `toneColors` and `healthStateColors`
 * resolve to those properties; light needs none because the light values are
 * the `var()` fallbacks. Components never branch on the theme.
 */
export function FleetThemeProvider({ children }: { children: ReactNode }) {
  const { trackEvent } = useFleetAnalytics();
  // Stored as the plain string `light` | `dark` | `auto` (not JSON) only once
  // the user explicitly picks a theme, so first-time visitors default to 'auto'
  // (following the browser/OS color-scheme preference).
  const [storedChoice, setStoredChoice] = useState<
    FleetThemeChoice | undefined
  >(() => readStoredThemeChoice());
  const prefersDark = useMediaQuery('(prefers-color-scheme: dark)', {
    noSsr: true,
  });

  useEffect(() => {
    const onStorage = (e: StorageEvent) => {
      if (e.key !== FLEET_THEME_STORAGE_KEY) return;
      setStoredChoice(isFleetThemeChoice(e.newValue) ? e.newValue : undefined);
    };
    window.addEventListener('storage', onStorage);
    return () => window.removeEventListener('storage', onStorage);
  }, []);

  const choice: FleetThemeChoice = storedChoice ?? 'auto';
  const resolvedId: FleetThemeId =
    choice === 'auto' ? (prefersDark ? 'dark' : 'light') : choice;
  const palette = FLEET_PALETTES[resolvedId];

  const muiTheme = useMemo(
    () => (palette.mode === 'light' ? lightTheme : createFleetTheme(palette)),
    [palette],
  );
  const globalStyles = useMemo(
    () =>
      palette.mode === 'dark'
        ? {
            ':root': {
              ...fleetThemeCssVars(palette),
              ...luciCommonCssVars(palette),
              colorScheme: 'dark',
            },
            'html, body': {
              backgroundColor: palette.colors.canvas,
              color: palette.colors.textPrimary,
            },
            ...codeMirrorDarkStyles(palette),
          }
        : null,
    [palette],
  );

  // Sync `data-fleet-theme` before child passive effects run so first-mount
  // analytics read the resolved theme.
  useLayoutEffect(() => {
    document.documentElement.dataset.fleetTheme = resolvedId;
    return () => {
      delete document.documentElement.dataset.fleetTheme;
    };
  }, [resolvedId]);

  useEffect(() => {
    window.gtag?.('set', 'user_properties', {
      fleet_theme: resolvedId,
      fleet_theme_choice: storedChoice ?? 'default',
    });
  }, [resolvedId, storedChoice]);

  const sessionTrackedRef = useRef(false);
  useEffect(() => {
    if (sessionTrackedRef.current) return;
    sessionTrackedRef.current = true;
    if (markSessionTracked()) {
      trackEvent('theme_session_active', {
        theme: resolvedId,
        themeChoice: storedChoice ?? 'default',
        themeResolved: resolvedId,
      });
    }
  }, [resolvedId, storedChoice, trackEvent]);

  const setChoice = useCallback(
    (next: FleetThemeChoice, source?: FleetThemeToggleSource) => {
      const nextResolved: FleetThemeId =
        next === 'auto' ? (prefersDark ? 'dark' : 'light') : next;
      writeStoredThemeChoice(next);
      setStoredChoice(next);
      trackEvent('theme_changed', {
        componentName: source ?? 'settings_menu',
        theme: nextResolved,
        themeChoice: next,
        themeResolved: nextResolved,
      });
    },
    [prefersDark, trackEvent],
  );

  const ctx = useMemo(
    () => ({ choice, palette, setChoice }),
    [choice, palette, setChoice],
  );

  return (
    <FleetThemeContext.Provider value={ctx}>
      <ThemeProvider theme={muiTheme}>
        {globalStyles && <GlobalStyles styles={globalStyles} />}
        {children}
      </ThemeProvider>
    </FleetThemeContext.Provider>
  );
}
