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

import ArrowForwardIcon from '@mui/icons-material/ArrowForward';
import CloseIcon from '@mui/icons-material/Close';
import ReplayIcon from '@mui/icons-material/Replay';
import StarIcon from '@mui/icons-material/Star';
import StarBorderIcon from '@mui/icons-material/StarBorder';
import TimerOffIcon from '@mui/icons-material/TimerOff';
import VolumeOffIcon from '@mui/icons-material/VolumeOff';
import VolumeUpIcon from '@mui/icons-material/VolumeUp';
import {
  Box,
  Button,
  Chip,
  IconButton,
  Tooltip,
  Typography,
} from '@mui/material';
import { useCallback, useEffect, useRef, useState } from 'react';

import happyBassSrc from '@/fleet/assets/pngs/fleet-console-mascot.png';
import sadBassSrc from '@/fleet/assets/pngs/sad_bass.png';
import { iconButtonSx, surfaceCardClippedSx } from '@/fleet/theme/recipes';
import {
  borders,
  fleetColors,
  fontFamilies,
  fontSizes,
  radii,
  statusTones,
  toneColors,
} from '@/fleet/theme/tokens';

import {
  computeStars,
  createGameState,
  formatRunTime,
  isDevTime,
  isTasTime,
  stepGame,
} from './engine';
import { LEVELS, WORLD_HEIGHT, WORLD_WIDTH } from './levels';
import { renderGame, RendererAssets } from './renderer';
import { tenSecondBassSound } from './sound';
import { loadStageRecords, saveStageRecord } from './storage';
import { GameState, InputSnapshot, StageRecord, StarRating } from './types';

interface RunOutcome {
  kind: 'cleared' | 'failed';
  timeMs: number;
  stars: StarRating;
  devMedal: boolean;
  tasMedal: boolean;
  isNewPb: boolean;
  deltaMs: number | null;
  shownAtMs: number;
}

const OUTCOME_INPUT_LOCKOUT_MS = 320;

function formatStarsBadge(stars: StarRating): string {
  return '★'.repeat(stars) + '☆'.repeat(3 - stars);
}

export interface TenSecondBassGameProps {
  onExit: () => void;
}

export const TenSecondBassGame = ({ onExit }: TenSecondBassGameProps) => {
  const [stageIndex, setStageIndex] = useState(0);
  const [records, setRecords] = useState<Record<string, StageRecord>>(() =>
    loadStageRecords(),
  );
  const [muted, setMuted] = useState(() => tenSecondBassSound.isMuted());
  const [outcome, setOutcome] = useState<RunOutcome | null>(null);

  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const stateRef = useRef<GameState>(createGameState(LEVELS[0]));
  const recordsRef = useRef(records);
  recordsRef.current = records;

  const assetsRef = useRef<RendererAssets>({
    sadBass: null,
    happyBass: null,
  });

  const keysHeldRef = useRef<Set<string>>(new Set());
  const pendingInputRef = useRef({
    downPressed: false,
    jumpPressed: false,
    slashPressed: false,
    shurikenPressed: false,
  });
  const stageIndexRef = useRef(stageIndex);
  stageIndexRef.current = stageIndex;
  const outcomeRef = useRef<RunOutcome | null>(outcome);
  outcomeRef.current = outcome;

  const startStage = useCallback((idx: number, playResetSound = false) => {
    const safeIdx = ((idx % LEVELS.length) + LEVELS.length) % LEVELS.length;
    const sameStage = safeIdx === stageIndexRef.current;
    const nextAttempt = sameStage ? stateRef.current.attemptCount + 1 : 1;
    setStageIndex(safeIdx);
    stageIndexRef.current = safeIdx;
    stateRef.current = createGameState(LEVELS[safeIdx], nextAttempt);
    pendingInputRef.current = {
      downPressed: false,
      jumpPressed: false,
      slashPressed: false,
      shurikenPressed: false,
    };
    setOutcome(null);
    outcomeRef.current = null;
    if (playResetSound) {
      tenSecondBassSound.playReset();
    }
  }, []);

  const handleToggleMute = useCallback(() => {
    setMuted(tenSecondBassSound.toggleMute());
  }, []);

  useEffect(() => {
    if (typeof Image === 'undefined') return;
    const sad = new Image();
    sad.src = sadBassSrc;
    const happy = new Image();
    happy.src = happyBassSrc;
    assetsRef.current = { sadBass: sad, happyBass: happy };
  }, []);

  // Capture-phase keyboard handler prevents Fleet global shortcuts (g d, g r, [, ])
  // from hijacking gameplay inputs.
  useEffect(() => {
    const GAME_CODES = new Set([
      'KeyA',
      'KeyD',
      'KeyW',
      'KeyS',
      'KeyJ',
      'KeyK',
      'KeyZ',
      'KeyX',
      'KeyR',
      'KeyG',
      'ArrowLeft',
      'ArrowRight',
      'ArrowUp',
      'ArrowDown',
      'Space',
      'Enter',
      'Escape',
    ]);

    const onKeyDown = (e: KeyboardEvent) => {
      if (!GAME_CODES.has(e.code)) return;

      e.preventDefault();
      e.stopPropagation();

      if (e.code === 'Escape') {
        onExit();
        return;
      }

      if (e.code === 'KeyR' && !e.repeat) {
        startStage(stageIndexRef.current, true);
        return;
      }

      if (
        outcomeRef.current &&
        (e.code === 'Space' || e.code === 'Enter') &&
        !e.repeat
      ) {
        // Prevent an in-flight jump press at the moment of clear from skipping results
        if (
          performance.now() - outcomeRef.current.shownAtMs <
          OUTCOME_INPUT_LOCKOUT_MS
        ) {
          return;
        }
        if (outcomeRef.current.kind === 'cleared') {
          const nextIdx =
            stageIndexRef.current + 1 < LEVELS.length
              ? stageIndexRef.current + 1
              : 0;
          startStage(nextIdx);
        } else {
          startStage(stageIndexRef.current, true);
        }
        return;
      }

      const wasHeld = keysHeldRef.current.has(e.code);
      keysHeldRef.current.add(e.code);

      if (!wasHeld) {
        if (e.code === 'Space' || e.code === 'KeyW' || e.code === 'ArrowUp') {
          pendingInputRef.current.jumpPressed = true;
        }
        if (e.code === 'KeyS' || e.code === 'ArrowDown') {
          pendingInputRef.current.downPressed = true;
        }
        if (e.code === 'KeyZ' || e.code === 'KeyJ') {
          pendingInputRef.current.slashPressed = true;
        }
        if (e.code === 'KeyX' || e.code === 'KeyK') {
          pendingInputRef.current.shurikenPressed = true;
        }
      }
    };

    const onKeyUp = (e: KeyboardEvent) => {
      keysHeldRef.current.delete(e.code);
    };

    const onBlur = () => {
      keysHeldRef.current.clear();
    };

    window.addEventListener('keydown', onKeyDown, { capture: true });
    window.addEventListener('keyup', onKeyUp);
    window.addEventListener('blur', onBlur);
    return () => {
      window.removeEventListener('keydown', onKeyDown, { capture: true });
      window.removeEventListener('keyup', onKeyUp);
      window.removeEventListener('blur', onBlur);
    };
  }, [onExit, startStage]);

  useEffect(() => {
    let rafId = 0;
    let lastTime = performance.now();
    let accumulator = 0;
    const FIXED_DT = 1 / 60;

    const frame = (now: number) => {
      const elapsedSec = Math.min(0.05, (now - lastTime) / 1000);
      lastTime = now;
      accumulator += elapsedSec;

      while (accumulator >= FIXED_DT) {
        accumulator -= FIXED_DT;
        const held = keysHeldRef.current;
        const snapshot: InputSnapshot = {
          left: held.has('KeyA') || held.has('ArrowLeft'),
          right: held.has('KeyD') || held.has('ArrowRight'),
          downHeld: held.has('KeyS') || held.has('ArrowDown'),
          downPressed: pendingInputRef.current.downPressed,
          jumpHeld:
            held.has('Space') || held.has('KeyW') || held.has('ArrowUp'),
          jumpPressed: pendingInputRef.current.jumpPressed,
          slashPressed: pendingInputRef.current.slashPressed,
          shurikenPressed: pendingInputRef.current.shurikenPressed,
        };
        pendingInputRef.current.downPressed = false;
        pendingInputRef.current.jumpPressed = false;
        pendingInputRef.current.slashPressed = false;
        pendingInputRef.current.shurikenPressed = false;

        const prevPhase = stateRef.current.phase;
        const pbSplits =
          recordsRef.current[stateRef.current.level.id]?.splitTimesMs;
        const events = stepGame(stateRef.current, snapshot, FIXED_DT, pbSplits);

        if (events.jump) tenSecondBassSound.playJump('ground');
        if (events.doubleJump) tenSecondBassSound.playJump('double');
        if (events.wallJump) tenSecondBassSound.playJump('wall');
        if (events.slash) tenSecondBassSound.playSlash();
        if (events.shurikenThrow) tenSecondBassSound.playShuriken();
        if (events.shurikenEmpty) tenSecondBassSound.playShurikenEmpty();
        if (events.shieldDeflect) tenSecondBassSound.playShieldDeflect();
        if (events.repairCount > 0) {
          tenSecondBassSound.playRepair(stateRef.current.comboCount);
        }
        if (events.countdownTick) tenSecondBassSound.playCountdownTick();
        if (events.hazardReset) tenSecondBassSound.playReset();

        if (events.stageClear && prevPhase !== 'cleared') {
          tenSecondBassSound.playStageClear();
          const st = stateRef.current;
          const timeMs = Math.round(st.elapsedMs);
          const stars = computeStars(st.level, timeMs);
          const devMedal = isDevTime(st.level, timeMs);
          const tasMedal = isTasTime(st.level, timeMs);
          const prevRec = recordsRef.current[st.level.id];
          const isNewPb = !prevRec || timeMs < prevRec.bestTimeMs;
          const deltaMs = prevRec ? timeMs - prevRec.bestTimeMs : null;

          if (isNewPb) {
            const updated = saveStageRecord(st.level.id, {
              bestTimeMs: timeMs,
              stars,
              devMedal,
              tasMedal,
              splitTimesMs: st.splitTimesMs,
              ghost: st.recording,
            });
            recordsRef.current = updated;
            setRecords(updated);
          }

          const nextOutcome: RunOutcome = {
            kind: 'cleared',
            timeMs,
            stars,
            devMedal,
            tasMedal,
            isNewPb,
            deltaMs,
            shownAtMs: performance.now(),
          };
          outcomeRef.current = nextOutcome;
          setOutcome(nextOutcome);
        } else if (events.timeUp && prevPhase !== 'failed') {
          tenSecondBassSound.playTimeUp();
          const nextOutcome: RunOutcome = {
            kind: 'failed',
            timeMs: 10_000,
            stars: 0,
            devMedal: false,
            tasMedal: false,
            isNewPb: false,
            deltaMs: null,
            shownAtMs: performance.now(),
          };
          outcomeRef.current = nextOutcome;
          setOutcome(nextOutcome);
        }
      }

      const canvas = canvasRef.current;
      if (canvas) {
        const ctx = canvas.getContext('2d');
        if (ctx) {
          const dpr = Math.min(window.devicePixelRatio || 1, 2);
          const targetW = Math.round(WORLD_WIDTH * dpr);
          const targetH = Math.round(WORLD_HEIGHT * dpr);
          if (canvas.width !== targetW || canvas.height !== targetH) {
            canvas.width = targetW;
            canvas.height = targetH;
          }
          ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
          renderGame(
            ctx,
            stateRef.current,
            assetsRef.current,
            recordsRef.current[stateRef.current.level.id],
            now,
          );
        }
      }

      rafId = window.requestAnimationFrame(frame);
    };

    rafId = window.requestAnimationFrame(frame);
    return () => window.cancelAnimationFrame(rafId);
  }, []);

  const currentLevel = LEVELS[stageIndex];
  const hasNextStage = stageIndex + 1 < LEVELS.length;
  const totalStars = LEVELS.reduce(
    (sum, lvl) => sum + (records[lvl.id]?.stars ?? 0),
    0,
  );
  const allCleared = LEVELS.every((lvl) => Boolean(records[lvl.id]));
  const totalBestMs = allCleared
    ? LEVELS.reduce((sum, lvl) => sum + (records[lvl.id]?.bestTimeMs ?? 0), 0)
    : null;

  const nextStarMessage = (() => {
    if (!outcome || outcome.kind !== 'cleared') return '';
    if (outcome.tasMedal) {
      return 'GEMINI TIME [TAS] BEATEN — WAIT, ARE YOU A BOT?!';
    }
    if (outcome.devMedal) {
      const gapMs = Math.max(0, outcome.timeMs - currentLevel.stars.tasTimeMs);
      return `DEV TIME CRUSHED! (${(gapMs / 1000).toFixed(2)}s off Gemini [TAS] — basically humanly impossible)`;
    }
    if (outcome.stars === 3) {
      const gapMs = Math.max(0, outcome.timeMs - currentLevel.stars.devTimeMs);
      return `3 STARS! (${(gapMs / 1000).toFixed(2)}s off Dev Time)`;
    }
    const targetMs =
      outcome.stars === 2
        ? currentLevel.stars.threeStarMs
        : currentLevel.stars.twoStarMs;
    const targetLabel = outcome.stars === 2 ? '★★★' : '★★';
    const gapSec = Math.max(0.01, (outcome.timeMs - targetMs) / 1000).toFixed(
      2,
    );
    return `Missed ${targetLabel} by ${gapSec}s (target <= ${formatRunTime(targetMs)}s)`;
  })();

  return (
    <Box
      sx={{
        width: '100%',
        maxWidth: 1008,
        mx: 'auto',
        px: 3,
        py: 2.5,
        display: 'flex',
        flexDirection: 'column',
        gap: 1.5,
      }}
    >
      {/* Top stage bar */}
      <Box
        sx={{
          ...surfaceCardClippedSx,
          px: 2,
          py: 1.25,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          flexWrap: 'wrap',
          gap: 1.25,
        }}
      >
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            gap: 1.25,
            flexWrap: 'wrap',
          }}
        >
          <Typography
            variant="subtitle1"
            sx={{
              fontFamily: fontFamilies.display,
              fontWeight: 700,
              color: fleetColors.textPrimary,
              letterSpacing: '0.03em',
            }}
          >
            10-SECOND BASS
          </Typography>
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              gap: 0.75,
              flexWrap: 'wrap',
            }}
          >
            {LEVELS.map((lvl, idx) => {
              const rec = records[lvl.id];
              const isSelected = idx === stageIndex;
              return (
                <Chip
                  key={lvl.id}
                  size="small"
                  label={
                    rec
                      ? `0${idx + 1} ${formatStarsBadge(rec.stars)}`
                      : `0${idx + 1} · ${(lvl.name.split('//')[1]?.trim() ?? lvl.name).split(' ').pop()}`
                  }
                  onClick={() => startStage(idx)}
                  aria-pressed={isSelected}
                  sx={{
                    fontFamily: fontFamilies.mono,
                    fontSize: fontSizes.caption,
                    fontWeight: isSelected ? 700 : 500,
                    borderRadius: radii.pill,
                    border: borders.default,
                    borderColor: isSelected
                      ? fleetColors.primary
                      : fleetColors.border,
                    backgroundColor: isSelected
                      ? fleetColors.primaryContainer
                      : rec?.tasMedal
                        ? toneColors.teal.container
                        : rec?.devMedal
                          ? statusTones.special.bg
                          : rec
                            ? statusTones.warning.bg
                            : fleetColors.surface,
                    color: isSelected
                      ? fleetColors.onPrimaryContainer
                      : rec?.tasMedal
                        ? toneColors.teal.onContainer
                        : rec?.devMedal
                          ? statusTones.special.fg
                          : rec
                            ? statusTones.warning.fg
                            : fleetColors.textSecondary,
                  }}
                />
              );
            })}
          </Box>
        </Box>

        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
          {totalBestMs !== null && (
            <Chip
              size="small"
              label={`ALL 5: ${formatRunTime(totalBestMs)}s`}
              sx={{
                fontFamily: fontFamilies.mono,
                fontSize: fontSizes.caption,
                fontWeight: 700,
                borderRadius: radii.pill,
                backgroundColor: statusTones.special.bg,
                color: statusTones.special.fg,
              }}
            />
          )}
          <Chip
            size="small"
            label={`★ ${totalStars}/${LEVELS.length * 3}`}
            sx={{
              fontFamily: fontFamilies.mono,
              fontSize: fontSizes.caption,
              fontWeight: 700,
              borderRadius: radii.pill,
              backgroundColor: statusTones.warning.bg,
              color: statusTones.warning.fg,
            }}
          />
          <Tooltip title="Restart stage (R)">
            <IconButton
              aria-label="Restart stage"
              onClick={() => startStage(stageIndex, true)}
              sx={iconButtonSx}
            >
              <ReplayIcon fontSize="small" />
            </IconButton>
          </Tooltip>
          <Tooltip title={muted ? 'Unmute sound' : 'Mute sound'}>
            <IconButton
              aria-label={muted ? 'Unmute sound' : 'Mute sound'}
              onClick={handleToggleMute}
              sx={iconButtonSx}
            >
              {muted ? (
                <VolumeOffIcon fontSize="small" />
              ) : (
                <VolumeUpIcon fontSize="small" />
              )}
            </IconButton>
          </Tooltip>
          <Tooltip title="Back to 404 (Esc)">
            <IconButton
              aria-label="Exit game"
              onClick={onExit}
              sx={iconButtonSx}
            >
              <CloseIcon fontSize="small" />
            </IconButton>
          </Tooltip>
        </Box>
      </Box>

      {/* Canvas Arena */}
      <Box
        sx={{
          ...surfaceCardClippedSx,
          position: 'relative',
          width: '100%',
          aspectRatio: '16 / 9',
          userSelect: 'none',
        }}
      >
        <canvas
          ref={canvasRef}
          aria-label="10-Second Bass speedrun arena"
          style={{
            display: 'block',
            width: '100%',
            height: '100%',
          }}
        />

        {/* Stage Clear / Time Up Overlay */}
        {outcome && (
          <Box
            role="dialog"
            aria-label={
              outcome.kind === 'cleared' ? 'Stage cleared' : 'Time up'
            }
            sx={{
              position: 'absolute',
              inset: 0,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              backgroundColor: fleetColors.scrim,
              p: 2,
            }}
          >
            <Box
              sx={{
                ...surfaceCardClippedSx,
                width: '100%',
                maxWidth: 420,
                p: 3,
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                gap: 1.5,
                textAlign: 'center',
              }}
            >
              {outcome.kind === 'cleared' ? (
                <>
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                    {[1, 2, 3].map((n) =>
                      n <= outcome.stars ? (
                        <StarIcon
                          key={n}
                          sx={{
                            fontSize: 36,
                            color: outcome.tasMedal
                              ? toneColors.teal.accent
                              : outcome.devMedal
                                ? statusTones.special.dot
                                : statusTones.warning.dot,
                          }}
                        />
                      ) : (
                        <StarBorderIcon
                          key={n}
                          sx={{
                            fontSize: 36,
                            color: fleetColors.borderEmphasis,
                          }}
                        />
                      ),
                    )}
                  </Box>

                  <Typography
                    variant="overline"
                    sx={{
                      fontFamily: fontFamilies.mono,
                      fontWeight: 700,
                      color: statusTones.success.fg,
                      letterSpacing: '0.1em',
                    }}
                  >
                    FLEET REPAIRED · {currentLevel.name}
                  </Typography>

                  <Typography
                    variant="h3"
                    sx={{
                      fontFamily: fontFamilies.mono,
                      fontWeight: 800,
                      color: fleetColors.textPrimary,
                    }}
                  >
                    {formatRunTime(outcome.timeMs)}s
                  </Typography>

                  <Box
                    sx={{
                      display: 'flex',
                      gap: 1,
                      flexWrap: 'wrap',
                      justifyContent: 'center',
                    }}
                  >
                    {outcome.tasMedal ? (
                      <Chip
                        size="small"
                        label="GEMINI TIME [TAS]"
                        sx={{
                          fontFamily: fontFamilies.mono,
                          fontWeight: 800,
                          borderRadius: radii.pill,
                          backgroundColor: toneColors.teal.container,
                          color: toneColors.teal.onContainer,
                        }}
                      />
                    ) : (
                      outcome.devMedal && (
                        <Chip
                          size="small"
                          label="DEV TIME"
                          sx={{
                            fontFamily: fontFamilies.mono,
                            fontWeight: 800,
                            borderRadius: radii.pill,
                            backgroundColor: statusTones.special.bg,
                            color: statusTones.special.fg,
                          }}
                        />
                      )
                    )}
                    {outcome.isNewPb && (
                      <Chip
                        size="small"
                        label="NEW PERSONAL BEST"
                        sx={{
                          fontFamily: fontFamilies.mono,
                          fontWeight: 700,
                          borderRadius: radii.pill,
                          backgroundColor: statusTones.success.bg,
                          color: statusTones.success.fg,
                        }}
                      />
                    )}
                    {outcome.deltaMs !== null && (
                      <Chip
                        size="small"
                        label={
                          outcome.deltaMs <= 0
                            ? `-${formatRunTime(Math.abs(outcome.deltaMs))}s vs PB`
                            : `+${formatRunTime(outcome.deltaMs)}s vs PB`
                        }
                        sx={{
                          fontFamily: fontFamilies.mono,
                          fontWeight: 600,
                          borderRadius: radii.pill,
                          backgroundColor:
                            outcome.deltaMs <= 0
                              ? statusTones.success.bg
                              : statusTones.warning.bg,
                          color:
                            outcome.deltaMs <= 0
                              ? statusTones.success.fg
                              : statusTones.warning.fg,
                        }}
                      />
                    )}
                  </Box>

                  <Typography
                    variant="body2"
                    sx={{
                      fontFamily: fontFamilies.mono,
                      color: outcome.tasMedal
                        ? toneColors.teal.main
                        : outcome.devMedal
                          ? statusTones.special.fg
                          : fleetColors.textSecondary,
                      fontWeight:
                        outcome.tasMedal || outcome.devMedal ? 700 : 500,
                    }}
                  >
                    {nextStarMessage}
                  </Typography>

                  <Box sx={{ display: 'flex', gap: 1.5, mt: 1 }}>
                    <Button
                      variant="outlined"
                      startIcon={<ReplayIcon />}
                      onClick={() => startStage(stageIndex, true)}
                    >
                      Retry (R)
                    </Button>
                    <Button
                      variant="contained"
                      endIcon={<ArrowForwardIcon />}
                      onClick={() =>
                        startStage(hasNextStage ? stageIndex + 1 : 0)
                      }
                    >
                      {hasNextStage
                        ? 'Next stage (Space)'
                        : 'Play stage 1 (Space)'}
                    </Button>
                  </Box>
                </>
              ) : (
                <>
                  <TimerOffIcon
                    sx={{ fontSize: 40, color: statusTones.error.fg }}
                  />
                  <Typography
                    variant="h4"
                    sx={{
                      fontFamily: fontFamilies.mono,
                      fontWeight: 800,
                      color: statusTones.error.fg,
                    }}
                  >
                    TIME UP!
                  </Typography>
                  <Typography
                    variant="body2"
                    sx={{
                      fontFamily: fontFamilies.text,
                      color: fleetColors.textSecondary,
                    }}
                  >
                    10 seconds elapsed before every broken DUT was repaired.
                  </Typography>
                  <Button
                    variant="contained"
                    startIcon={<ReplayIcon />}
                    onClick={() => startStage(stageIndex, true)}
                    sx={{ mt: 1 }}
                  >
                    Try again (R / Space)
                  </Button>
                </>
              )}
            </Box>
          </Box>
        )}
      </Box>

      {/* Pure keyboard controls bar */}
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          flexWrap: 'wrap',
          gap: 1,
          px: 0.5,
        }}
      >
        <Typography
          variant="caption"
          sx={{
            fontFamily: fontFamilies.mono,
            color: fleetColors.textSecondary,
          }}
        >
          {currentLevel.subtitle}
        </Typography>
        <Typography
          variant="caption"
          sx={{
            fontFamily: fontFamilies.mono,
            color: fleetColors.textTertiary,
          }}
        >
          A/D or Arrows Move · W/Up/Space Double-Jump · S/Down Drop & Fast-Fall
          · Z/J Manual Repair · X/K Auto-Repair (3) · R Restart
        </Typography>
      </Box>
    </Box>
  );
};
