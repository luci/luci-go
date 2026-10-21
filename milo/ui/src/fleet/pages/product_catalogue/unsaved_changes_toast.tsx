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

import AutoAwesomeIcon from '@mui/icons-material/AutoAwesome';
import CheckIcon from '@mui/icons-material/Check';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import ErrorOutlineIcon from '@mui/icons-material/ErrorOutline';
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import RestartAltIcon from '@mui/icons-material/RestartAlt';
import { Box, Button, Paper, Typography } from '@mui/material';

import { StatusPill } from '@/fleet/components/status_pill/status_pill';
import {
  controlHeights,
  fleetColors,
  fontSizes,
  iconSizes,
  radii,
  statusTones,
} from '@/fleet/theme/tokens';

import {
  CAPTAIN_FIN_CHAT_URL,
  buildJetskiPrefilledPromptUrl,
} from './catalog_edit_validation';

export interface UnsavedChangesToastProps {
  readonly dirtyCount: number;
  readonly validationErrorCount?: number;
  readonly errorSummaryText?: string;
  readonly summaryText?: string;
  readonly productCatalogId?: string;
  readonly prompt?: string;
  readonly copiedPrompt?: boolean;
  readonly onDiscard: () => void;
  readonly onCopyPrompt: () => void;
}

/**
 * Always-visible fixed bottom bar shown whenever the user has staged edits on
 * a Product Catalog entry. Clicking "Copy prompt to save" copies the prompt
 * directly to the clipboard and surfaces an inline confirmation notice with
 * 1-click links to `@Captain Fin` (Google Chat) and Jetski without opening a
 * separate modal.
 */
export const UnsavedChangesToast = ({
  dirtyCount,
  validationErrorCount = 0,
  errorSummaryText,
  summaryText,
  productCatalogId,
  prompt = '',
  copiedPrompt = false,
  onDiscard,
  onCopyPrompt,
}: UnsavedChangesToastProps) => {
  if (dirtyCount <= 0 && validationErrorCount <= 0) {
    return null;
  }

  const hasErrors = validationErrorCount > 0;
  const isCopiedState = !hasErrors && copiedPrompt;
  const activeBorderColor = hasErrors
    ? statusTones.error.fg
    : isCopiedState
      ? statusTones.success.fg
      : fleetColors.primary;

  return (
    <Paper
      role="region"
      aria-label="Unsaved catalog edits"
      data-testid="unsaved-changes-toast"
      elevation={4}
      sx={{
        position: 'fixed',
        bottom: 24,
        left: '50%',
        transform: 'translateX(-50%)',
        zIndex: 1200,
        width: 'max-content',
        maxWidth: 'calc(100vw - 48px)',
        backgroundColor: fleetColors.surface,
        border: `1px solid ${activeBorderColor}`,
        borderRadius: radii.md,
        px: 2,
        py: 1.25,
        display: 'flex',
        flexWrap: 'wrap',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: 2,
      }}
    >
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.25 }}>
        <StatusPill
          tone={hasErrors ? 'error' : isCopiedState ? 'success' : 'info'}
          size="compact"
          dot={!hasErrors && !isCopiedState}
        >
          {hasErrors
            ? `${validationErrorCount} validation ${validationErrorCount === 1 ? 'error' : 'errors'}`
            : isCopiedState
              ? 'Copied prompt'
              : `${dirtyCount} unsaved ${dirtyCount === 1 ? 'change' : 'changes'}`}
        </StatusPill>

        <Typography
          variant="body2"
          data-testid={isCopiedState ? 'handoff-guidance' : undefined}
          sx={{
            color: hasErrors
              ? statusTones.error.fg
              : isCopiedState
                ? statusTones.success.fg
                : fleetColors.textPrimary,
            fontSize: fontSizes.label,
            fontWeight: 500,
            display: 'flex',
            alignItems: 'center',
            gap: 0.75,
          }}
        >
          {hasErrors && (
            <ErrorOutlineIcon sx={{ fontSize: iconSizes.control }} />
          )}
          {isCopiedState && <CheckIcon sx={{ fontSize: iconSizes.control }} />}
          {hasErrors
            ? (errorSummaryText ??
              'Fix highlighted fields before copying a prompt.')
            : isCopiedState
              ? [
                  `Copied prompt${productCatalogId ? ` for ${productCatalogId}` : ''}!`,
                  'Paste into @Captain Fin or Jetski to create the CL.',
                ].join(' ')
              : (summaryText ??
                'Copy a prompt for @Captain Fin or Jetski to save.')}
        </Typography>
      </Box>

      {/* Hidden pre element preserving test/inspector access to the generated prompt */}
      {isCopiedState && prompt && (
        <Box
          component="pre"
          data-testid="generated-catalog-prompt"
          sx={{ display: 'none' }}
        >
          {prompt}
        </Box>
      )}

      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
        <Button
          variant="outlined"
          size="small"
          onClick={onDiscard}
          data-testid="discard-catalog-edits-btn"
          startIcon={<RestartAltIcon sx={{ fontSize: iconSizes.control }} />}
          sx={{
            minHeight: controlHeights.button,
            borderRadius: radii.pill,
            fontSize: fontSizes.label,
            textTransform: 'none',
            borderColor: fleetColors.border,
            color: fleetColors.textSecondary,
          }}
        >
          Discard
        </Button>

        {isCopiedState && (
          <>
            <Button
              variant="outlined"
              size="small"
              component="a"
              href={CAPTAIN_FIN_CHAT_URL}
              target="_blank"
              rel="noopener noreferrer"
              onClick={onCopyPrompt}
              endIcon={<OpenInNewIcon sx={{ fontSize: iconSizes.control }} />}
              data-testid="open-captain-fin-btn"
              sx={{
                minHeight: controlHeights.button,
                borderRadius: radii.pill,
                fontSize: fontSizes.label,
                textTransform: 'none',
                borderColor: fleetColors.border,
                color: fleetColors.textPrimary,
              }}
            >
              @Captain Fin
            </Button>
            <Button
              variant="outlined"
              size="small"
              component="a"
              href={buildJetskiPrefilledPromptUrl(prompt)}
              target="_blank"
              rel="noopener noreferrer"
              onClick={onCopyPrompt}
              endIcon={<OpenInNewIcon sx={{ fontSize: iconSizes.control }} />}
              data-testid="open-in-jetski-btn"
              sx={{
                minHeight: controlHeights.button,
                borderRadius: radii.pill,
                fontSize: fontSizes.label,
                textTransform: 'none',
                borderColor: fleetColors.border,
                color: fleetColors.textPrimary,
              }}
            >
              Jetski
            </Button>
          </>
        )}

        <Button
          variant="contained"
          size="small"
          disabled={hasErrors || dirtyCount <= 0}
          onClick={onCopyPrompt}
          startIcon={
            isCopiedState ? (
              <ContentCopyIcon sx={{ fontSize: iconSizes.control }} />
            ) : (
              <AutoAwesomeIcon sx={{ fontSize: iconSizes.control }} />
            )
          }
          data-testid="review-generate-prompt-btn"
          sx={{
            minHeight: controlHeights.button,
            borderRadius: radii.pill,
            fontSize: fontSizes.label,
            textTransform: 'none',
            backgroundColor: fleetColors.primary,
            '&:hover': {
              backgroundColor: fleetColors.primaryHover,
            },
          }}
        >
          {isCopiedState ? 'Copy again' : 'Copy prompt to save'}
        </Button>
      </Box>
    </Paper>
  );
};
