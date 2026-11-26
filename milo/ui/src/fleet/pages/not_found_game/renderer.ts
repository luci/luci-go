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

import {
  fleetColors,
  fontFamilies,
  statusTones,
  toneColors,
} from '@/fleet/theme/tokens';

import {
  computeStars,
  formatCountdown,
  formatRunTime,
  isDevTime,
  isTasTime,
  sampleGhostFrame,
} from './engine';
import { MAX_SHURIKENS, WORLD_HEIGHT, WORLD_WIDTH } from './levels';
import { DeviceState, GameState, GhostFrame, StageRecord } from './types';

export interface RendererAssets {
  sadBass: HTMLImageElement | null;
  happyBass: HTMLImageElement | null;
}

const CSS_VAR_RE = /^var\(\s*(--[^,\s)]+)\s*(?:,\s*(.+))?\)$/;

/**
 * Resolves a Fleet CSS-var color token (e.g. `var(--fc-surface, #ffffff)`)
 * against the canvas element's computed styles so Canvas 2D follows the active
 * light or dark Fleet theme automatically without branching on theme mode.
 */
export function resolveCanvasColor(
  styles: Pick<CSSStyleDeclaration, 'getPropertyValue'> | null,
  token: string,
): string {
  const trimmed = token.trim();
  const match = CSS_VAR_RE.exec(trimmed);
  if (!match) return trimmed;
  const live = styles?.getPropertyValue(match[1]).trim();
  if (live) return live;
  return match[2]?.trim() || trimmed;
}

type ColorResolver = (token: string) => string;

function createColorResolver(ctx: CanvasRenderingContext2D): ColorResolver {
  const styles =
    typeof window !== 'undefined' && ctx.canvas
      ? window.getComputedStyle(ctx.canvas)
      : null;
  const cache = new Map<string, string>();
  return (token: string) => {
    const cached = cache.get(token);
    if (cached !== undefined) return cached;
    const resolved = resolveCanvasColor(styles, token);
    cache.set(token, resolved);
    return resolved;
  };
}

function drawRoundedRect(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  w: number,
  h: number,
  r: number,
): void {
  const radius = Math.min(r, w / 2, h / 2);
  ctx.beginPath();
  ctx.moveTo(x + radius, y);
  ctx.lineTo(x + w - radius, y);
  ctx.quadraticCurveTo(x + w, y, x + w, y + radius);
  ctx.lineTo(x + w, y + h - radius);
  ctx.quadraticCurveTo(x + w, y + h, x + w - radius, y + h);
  ctx.lineTo(x + radius, y + h);
  ctx.quadraticCurveTo(x, y + h, x, y + h - radius);
  ctx.lineTo(x, y + radius);
  ctx.quadraticCurveTo(x, y, x + radius, y);
  ctx.closePath();
}

/** Spinning Auto-Repair gear/cog projectile & HUD icon. */
function drawAutoRepairGear(
  ctx: CanvasRenderingContext2D,
  c: ColorResolver,
  cx: number,
  cy: number,
  outerR: number,
  rotation: number,
  fill: string,
  stroke?: string,
  alpha = 1,
): void {
  ctx.save();
  ctx.globalAlpha *= alpha;
  ctx.translate(cx, cy);
  ctx.rotate(rotation);
  const teeth = 6;
  const innerR = outerR * 0.62;
  ctx.beginPath();
  for (let i = 0; i < teeth * 2; i++) {
    const r = i % 2 === 0 ? outerR : innerR;
    const angle = (Math.PI * i) / teeth;
    const px = Math.cos(angle) * r;
    const py = Math.sin(angle) * r;
    if (i === 0) ctx.moveTo(px, py);
    else ctx.lineTo(px, py);
  }
  ctx.closePath();
  ctx.fillStyle = c(fill);
  ctx.fill();
  if (stroke) {
    ctx.strokeStyle = c(stroke);
    ctx.lineWidth = 1.5;
    ctx.stroke();
  }
  ctx.fillStyle = c(fleetColors.surface);
  ctx.beginPath();
  ctx.arc(0, 0, outerR * 0.28, 0, Math.PI * 2);
  ctx.fill();
  ctx.restore();
}

function drawMiniStar(
  ctx: CanvasRenderingContext2D,
  c: ColorResolver,
  cx: number,
  cy: number,
  outerR: number,
  fill: string,
  alpha = 1,
): void {
  ctx.save();
  ctx.globalAlpha *= alpha;
  ctx.translate(cx, cy);
  ctx.beginPath();
  const points = 5;
  const innerR = outerR * 0.45;
  for (let i = 0; i < points * 2; i++) {
    const r = i % 2 === 0 ? outerR : innerR;
    const angle = (Math.PI * i) / points - Math.PI / 2;
    const px = Math.cos(angle) * r;
    const py = Math.sin(angle) * r;
    if (i === 0) ctx.moveTo(px, py);
    else ctx.lineTo(px, py);
  }
  ctx.closePath();
  ctx.fillStyle = c(fill);
  ctx.fill();
  ctx.restore();
}

function drawBackground(
  ctx: CanvasRenderingContext2D,
  c: ColorResolver,
  state: GameState,
): void {
  const sky = ctx.createLinearGradient(0, 0, 0, WORLD_HEIGHT);
  sky.addColorStop(0, c(fleetColors.infoBanner));
  sky.addColorStop(0.55, c(fleetColors.canvas));
  sky.addColorStop(1, c(fleetColors.surface));
  ctx.fillStyle = sky;
  ctx.fillRect(0, 0, WORLD_WIDTH, WORLD_HEIGHT);

  ctx.save();
  ctx.globalAlpha = 0.07;
  ctx.strokeStyle = c(fleetColors.primary);
  ctx.lineWidth = 1;
  ctx.beginPath();
  for (let x = 0; x <= WORLD_WIDTH; x += 40) {
    ctx.moveTo(x, 0);
    ctx.lineTo(x, WORLD_HEIGHT);
  }
  for (let y = 0; y <= WORLD_HEIGHT; y += 40) {
    ctx.moveTo(0, y);
    ctx.lineTo(WORLD_WIDTH, y);
  }
  ctx.stroke();
  ctx.restore();

  const cleared = state.phase === 'cleared';
  ctx.save();
  ctx.globalAlpha = cleared ? 0.14 : 0.07;
  ctx.fillStyle = c(cleared ? statusTones.success.dot : fleetColors.primary);
  ctx.font = `900 220px ${fontFamilies.display}`;
  ctx.textAlign = 'center';
  ctx.textBaseline = 'middle';
  ctx.fillText(cleared ? '200' : '404', WORLD_WIDTH / 2, WORLD_HEIGHT / 2 + 12);
  ctx.restore();
}

function drawPlatformsAndHazards(
  ctx: CanvasRenderingContext2D,
  c: ColorResolver,
  state: GameState,
  nowSec: number,
): void {
  for (const plat of state.level.platforms) {
    if (plat.oneWay) {
      // Visually distinct pass-through catwalk: slim tonal bar, dashed rail, drop chevrons
      ctx.save();
      ctx.globalAlpha = 0.92;
      ctx.fillStyle = c(fleetColors.primaryContainer);
      drawRoundedRect(ctx, plat.x, plat.y, plat.w, plat.h, 4);
      ctx.fill();
      ctx.globalAlpha = 1;

      ctx.strokeStyle = c(statusTones.info.dot);
      ctx.lineWidth = 1.75;
      drawRoundedRect(ctx, plat.x, plat.y, plat.w, plat.h, 4);
      ctx.stroke();

      ctx.strokeStyle = c(fleetColors.borderHover);
      ctx.lineWidth = 2.5;
      ctx.setLineDash([8, 5]);
      ctx.beginPath();
      ctx.moveTo(plat.x + 6, plat.y + 2);
      ctx.lineTo(plat.x + plat.w - 6, plat.y + 2);
      ctx.stroke();
      ctx.setLineDash([]);

      ctx.globalAlpha = 0.55;
      ctx.strokeStyle = c(statusTones.info.dot);
      ctx.lineWidth = 1.4;
      const step = 36;
      for (let cx = plat.x + 22; cx < plat.x + plat.w - 16; cx += step) {
        ctx.beginPath();
        ctx.moveTo(cx - 3.5, plat.y + 6);
        ctx.lineTo(cx, plat.y + 9.5);
        ctx.lineTo(cx + 3.5, plat.y + 6);
        ctx.stroke();
      }
      ctx.restore();
      continue;
    }

    // Solid architectural block (floors, start decks, wall-jumpable server columns)
    ctx.save();
    ctx.globalAlpha = 0.22;
    ctx.fillStyle = c(fleetColors.scrim);
    drawRoundedRect(ctx, plat.x + 2, plat.y + 4, plat.w, plat.h, 5);
    ctx.fill();
    ctx.restore();

    ctx.fillStyle = c(fleetColors.primaryContainerHover);
    drawRoundedRect(ctx, plat.x, plat.y, plat.w, plat.h, 5);
    ctx.fill();

    if (plat.h > plat.w * 1.5) {
      ctx.fillStyle = c(fleetColors.primary);
      ctx.fillRect(plat.x, plat.y + 6, 3, plat.h - 12);
      ctx.fillRect(plat.x + plat.w - 3, plat.y + 6, 3, plat.h - 12);
    } else {
      ctx.fillStyle = c(fleetColors.primary);
      ctx.fillRect(plat.x + 5, plat.y, plat.w - 10, 3);
    }

    ctx.strokeStyle = c(fleetColors.textPrimary);
    ctx.lineWidth = 2.2;
    drawRoundedRect(ctx, plat.x, plat.y, plat.w, plat.h, 5);
    ctx.stroke();
  }

  // Red power-fault laser hazards
  for (const h of state.level.hazards) {
    const pulse = 0.72 + Math.sin(nowSec * 14) * 0.25;
    ctx.save();
    ctx.globalAlpha = 0.34;
    ctx.fillStyle = c(statusTones.error.dot);
    drawRoundedRect(ctx, h.x - 4, h.y - 4, h.w + 8, h.h + 8, 6);
    ctx.fill();

    ctx.globalAlpha = 1;
    ctx.fillStyle = c(statusTones.error.dot);
    drawRoundedRect(ctx, h.x, h.y, h.w, h.h, 4);
    ctx.fill();

    ctx.globalAlpha = pulse;
    ctx.fillStyle = c(statusTones.error.bg);
    if (h.w > h.h) {
      ctx.fillRect(h.x + 4, h.y + h.h / 2 - 2, h.w - 8, 4);
    } else {
      ctx.fillRect(h.x + h.w / 2 - 2, h.y + 4, 4, h.h - 8);
    }
    ctx.restore();
  }
}

function drawDevice(
  ctx: CanvasRenderingContext2D,
  c: ColorResolver,
  dev: DeviceState,
  nowSec: number,
): void {
  const x = dev.x;
  const y = dev.y;

  ctx.save();
  if (dev.repaired) {
    ctx.globalAlpha = 0.4 + dev.repairFlash * 0.6;
  }

  const isShielded = Boolean(dev.shielded) && !dev.repaired;
  const tone = dev.repaired
    ? statusTones.success
    : isShielded
      ? statusTones.warning
      : statusTones.error;

  ctx.save();
  ctx.globalAlpha *= 0.3;
  ctx.fillStyle = c(tone.dot);
  ctx.beginPath();
  ctx.arc(
    x,
    y,
    dev.repaired ? 19 : 23 + Math.sin(nowSec * 7 + dev.bobOffset) * 2,
    0,
    Math.PI * 2,
  );
  ctx.fill();
  ctx.restore();

  if (isShielded) {
    ctx.strokeStyle = c(tone.dot);
    ctx.lineWidth = 2.5;
    ctx.setLineDash([7, 4]);
    ctx.beginPath();
    ctx.arc(x, y, 24, nowSec * 1.4, nowSec * 1.4 + Math.PI * 2);
    ctx.stroke();
    ctx.setLineDash([]);
  }

  ctx.fillStyle = c(tone.bg);
  ctx.strokeStyle = c(tone.dot);
  ctx.lineWidth = 2.4;

  if (dev.kind === 'chromebook') {
    drawRoundedRect(ctx, x - 15, y - 12, 30, 19, 3);
    ctx.fill();
    ctx.stroke();
    drawRoundedRect(ctx, x - 19, y + 7, 38, 5, 2);
    ctx.fill();
    ctx.stroke();
  } else if (dev.kind === 'pixel') {
    drawRoundedRect(ctx, x - 10, y - 16, 20, 32, 4);
    ctx.fill();
    ctx.stroke();
    ctx.fillStyle = c(tone.dot);
    ctx.fillRect(x - 6, y - 11, 12, 3);
  } else {
    drawRoundedRect(ctx, x - 15, y - 14, 30, 28, 4);
    ctx.fill();
    ctx.stroke();
    ctx.beginPath();
    ctx.moveTo(x - 10, y - 4);
    ctx.lineTo(x + 10, y - 4);
    ctx.moveTo(x - 10, y + 4);
    ctx.lineTo(x + 10, y + 4);
    ctx.stroke();
  }

  ctx.strokeStyle = c(tone.fg);
  ctx.lineWidth = 2.5;
  ctx.lineCap = 'round';
  ctx.lineJoin = 'round';
  if (dev.repaired) {
    ctx.beginPath();
    ctx.moveTo(x - 5, y);
    ctx.lineTo(x - 1, y + 4);
    ctx.lineTo(x + 6, y - 4);
    ctx.stroke();
  } else {
    ctx.beginPath();
    ctx.moveTo(x - 4, y - 4);
    ctx.lineTo(x + 4, y + 4);
    ctx.moveTo(x + 4, y - 4);
    ctx.lineTo(x - 4, y + 4);
    ctx.stroke();
  }

  if (!dev.repaired) {
    ctx.font = `700 8.5px ${fontFamilies.mono}`;
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    const label = isShielded ? 'MANUAL REPAIR' : 'NEEDS REPAIR';
    const pillW = isShielded ? 76 : 70;
    ctx.fillStyle = c(isShielded ? tone.fg : tone.dot);
    drawRoundedRect(ctx, x - pillW / 2, y - 33, pillW, 14, 7);
    ctx.fill();
    ctx.fillStyle = c(fleetColors.surface);
    ctx.fillText(label, x, y - 26);
  }

  ctx.restore();
}

function drawNinjaBass(
  ctx: CanvasRenderingContext2D,
  c: ColorResolver,
  assets: RendererAssets,
  x: number,
  y: number,
  w: number,
  h: number,
  facing: 1 | -1,
  happy: boolean,
  scaleX: number,
  scaleY: number,
  tiltRad: number,
  nowSec: number,
  alpha = 1,
): void {
  const img = happy ? assets.happyBass : assets.sadBass;
  const cx = x + w / 2;
  const cy = y + h / 2;
  const drawSize = 40;

  ctx.save();
  ctx.globalAlpha = alpha;
  ctx.translate(cx, cy);
  ctx.rotate(tiltRad);
  ctx.scale(-facing * scaleX, scaleY);

  if (img && img.complete && img.naturalWidth > 0) {
    ctx.drawImage(img, -drawSize / 2, -drawSize / 2, drawSize, drawSize);
  } else {
    ctx.fillStyle = c(happy ? statusTones.success.dot : fleetColors.primary);
    ctx.beginPath();
    ctx.arc(0, 0, 15, 0, Math.PI * 2);
    ctx.fill();
  }

  // Ninja red headband + fluttering ribbons
  ctx.strokeStyle = c(statusTones.error.dot);
  ctx.lineWidth = 3.5;
  ctx.lineCap = 'round';
  ctx.beginPath();
  ctx.moveTo(-11, -7);
  ctx.quadraticCurveTo(0, -10, 11, -6);
  ctx.stroke();

  const flutter = Math.sin(nowSec * 18) * 3.2;
  ctx.lineWidth = 2.6;
  ctx.beginPath();
  ctx.moveTo(10, -6);
  ctx.quadraticCurveTo(18, -9 + flutter, 25, -5 - flutter * 0.6);
  ctx.stroke();
  ctx.lineWidth = 2;
  ctx.beginPath();
  ctx.moveTo(10, -5);
  ctx.quadraticCurveTo(17, -2 - flutter * 0.7, 23, 1 + flutter * 0.8);
  ctx.stroke();

  ctx.restore();
}

function drawSlashArc(
  ctx: CanvasRenderingContext2D,
  c: ColorResolver,
  state: GameState,
): void {
  const p = state.player;
  if (p.slashTimer <= 0) return;

  const progress = 1 - p.slashTimer / 0.13;
  const cx = p.x + p.w / 2 + p.slashFacing * 10;
  const cy = p.y + p.h / 2;

  ctx.save();
  ctx.translate(cx, cy);
  ctx.scale(p.slashFacing, 1);
  ctx.globalAlpha = Math.max(0, 1 - progress * 0.85);

  ctx.save();
  ctx.globalAlpha *= 0.45;
  ctx.strokeStyle = c(fleetColors.borderHover);
  ctx.lineWidth = 18 * (1 - progress * 0.45);
  ctx.lineCap = 'round';
  ctx.beginPath();
  ctx.arc(4, 0, 44, -Math.PI * 0.44, Math.PI * 0.44);
  ctx.stroke();
  ctx.restore();

  ctx.strokeStyle = c(statusTones.info.dot);
  ctx.lineWidth = 9 * (1 - progress * 0.4);
  ctx.lineCap = 'round';
  ctx.beginPath();
  ctx.arc(6, 0, 44, -Math.PI * 0.42, Math.PI * 0.42);
  ctx.stroke();

  ctx.strokeStyle = c(fleetColors.surface);
  ctx.lineWidth = 4;
  ctx.beginPath();
  ctx.arc(8, 0, 44, -Math.PI * 0.36, Math.PI * 0.36);
  ctx.stroke();

  // Crisp Manual-Repair wrench silhouette sweeping along the arc
  ctx.save();
  const wrenchAngle = -0.85 + progress * 1.75;
  ctx.rotate(wrenchAngle);
  ctx.strokeStyle = c(fleetColors.textPrimary);
  ctx.lineWidth = 5;
  ctx.beginPath();
  ctx.moveTo(14, 0);
  ctx.lineTo(36, 0);
  ctx.stroke();
  ctx.strokeStyle = c(fleetColors.surface);
  ctx.lineWidth = 2.6;
  ctx.beginPath();
  ctx.moveTo(15, 0);
  ctx.lineTo(35, 0);
  ctx.stroke();
  ctx.strokeStyle = c(fleetColors.surface);
  ctx.lineWidth = 2.8;
  ctx.beginPath();
  ctx.arc(40, 0, 5.5, 0.55, Math.PI * 2 - 0.55);
  ctx.stroke();
  ctx.restore();

  ctx.restore();
}

function drawGhost(
  ctx: CanvasRenderingContext2D,
  c: ColorResolver,
  assets: RendererAssets,
  ghost: readonly GhostFrame[],
  elapsedMs: number,
  nowSec: number,
): void {
  const frame = sampleGhostFrame(ghost, elapsedMs);
  if (!frame) return;

  ctx.save();
  drawNinjaBass(
    ctx,
    c,
    assets,
    frame.x,
    frame.y,
    28,
    28,
    frame.facing,
    false,
    1,
    1,
    0,
    nowSec,
    0.26,
  );

  ctx.globalAlpha = 0.55;
  ctx.font = `800 9px ${fontFamilies.mono}`;
  ctx.textAlign = 'center';
  ctx.fillStyle = c(fleetColors.textSecondary);
  ctx.fillText('PB', frame.x + 14, frame.y - 8);
  ctx.restore();
}

function drawDarkHudBox(
  ctx: CanvasRenderingContext2D,
  c: ColorResolver,
  x: number,
  y: number,
  w: number,
  h: number,
  r: number,
  strokeColor: string = fleetColors.scrimHover,
  lineWidth = 1.5,
): void {
  ctx.save();
  ctx.fillStyle = c(fleetColors.scrim);
  drawRoundedRect(ctx, x, y, w, h, r);
  ctx.fill();
  ctx.restore();

  ctx.strokeStyle = c(strokeColor);
  ctx.lineWidth = lineWidth;
  drawRoundedRect(ctx, x, y, w, h, r);
  ctx.stroke();
}

function drawHud(
  ctx: CanvasRenderingContext2D,
  c: ColorResolver,
  state: GameState,
  record: StageRecord | undefined,
  nowSec: number,
): void {
  ctx.save();

  const nearClock =
    state.player.y < 86 &&
    state.player.x + state.player.w > WORLD_WIDTH / 2 - 120 &&
    state.player.x < WORLD_WIDTH / 2 + 120;

  // Top-center 10-Second countdown clock + live 3-Star pace strip + 3 Auto-Repair gears
  const clockW = 248;
  const clockH = 68;
  const clockX = WORLD_WIDTH / 2 - clockW / 2;
  const clockY = 10;

  ctx.save();
  if (nearClock) ctx.globalAlpha = 0.48;
  drawDarkHudBox(
    ctx,
    c,
    clockX,
    clockY,
    clockW,
    clockH,
    12,
    state.emptyStarFlashTimer > 0
      ? statusTones.error.dot
      : fleetColors.scrimHover,
    state.emptyStarFlashTimer > 0 ? 2 : 1.5,
  );

  const liveStars = computeStars(state.level, state.elapsedMs);
  const onTasPace = isTasTime(state.level, state.elapsedMs);
  const onDevPace = isDevTime(state.level, state.elapsedMs);
  const urgent = state.remainingMs <= 2000 && state.phase === 'running';
  const warn =
    state.phase === 'running' && (liveStars <= 1 || state.remainingMs <= 4000);
  const timerColor =
    state.phase === 'cleared'
      ? statusTones.success.dot
      : urgent
        ? statusTones.error.dot
        : warn
          ? statusTones.warning.dot
          : fleetColors.onScrim;

  ctx.fillStyle = c(timerColor);
  ctx.font = `900 ${urgent ? 27 + Math.sin(nowSec * 24) * 1.5 : 27}px ${fontFamilies.mono}`;
  ctx.textAlign = 'center';
  ctx.textBaseline = 'top';
  ctx.fillText(formatCountdown(state.remainingMs), WORLD_WIDTH / 2, clockY + 6);

  // Left half under clock: Live 3-Star Pace Strip
  for (let i = 0; i < 3; i++) {
    const sx = clockX + 34 + i * 22;
    const sy = clockY + 51;
    const active = i < liveStars;
    drawMiniStar(
      ctx,
      c,
      sx,
      sy,
      7.5,
      active
        ? onTasPace
          ? toneColors.teal.accent
          : onDevPace
            ? statusTones.special.dot
            : statusTones.warning.dot
        : fleetColors.onScrim,
      active ? 1 : 0.25,
    );
  }

  ctx.fillStyle = c(fleetColors.scrimHover);
  ctx.fillRect(WORLD_WIDTH / 2 - 1, clockY + 42, 2, 18);

  // Right half under clock: 3 Auto-Repair gear charges
  const emptyShake =
    state.emptyStarFlashTimer > 0 ? Math.sin(nowSec * 70) * 3 : 0;
  for (let i = 0; i < MAX_SHURIKENS; i++) {
    const gx = clockX + clockW - 78 + i * 24 + emptyShake;
    const gy = clockY + 51;
    const available = i < state.shurikensLeft;
    drawAutoRepairGear(
      ctx,
      c,
      gx,
      gy,
      8.2,
      available ? nowSec * 1.8 + i : 0,
      available
        ? statusTones.warning.dot
        : state.emptyStarFlashTimer > 0
          ? statusTones.error.dot
          : fleetColors.onScrim,
      undefined,
      available || state.emptyStarFlashTimer > 0 ? 1 : 0.25,
    );
  }
  ctx.restore();

  // Live PB split delta pill directly under the clock
  if (state.liveSplit && state.phase === 'running') {
    const ahead = state.liveSplit.deltaMs <= 0;
    const sign = ahead ? '-' : '+';
    const label = `${sign}${(Math.abs(state.liveSplit.deltaMs) / 1000).toFixed(2)}s vs PB`;
    ctx.save();
    ctx.globalAlpha = Math.min(1, state.liveSplit.timer * 2);
    drawDarkHudBox(
      ctx,
      c,
      WORLD_WIDTH / 2 - 58,
      clockY + clockH + 6,
      116,
      20,
      10,
    );
    ctx.fillStyle = c(
      ahead ? statusTones.success.dot : statusTones.warning.dot,
    );
    ctx.font = `800 10.5px ${fontFamilies.mono}`;
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText(label, WORLD_WIDTH / 2, clockY + clockH + 16);
    ctx.restore();
  }

  // Top-left: PB, 3-star target, & attempt counter
  const showTries = state.attemptCount > 1;
  drawDarkHudBox(ctx, c, 14, 12, 174, showTries ? 56 : 42, 10);

  ctx.fillStyle = c(fleetColors.onScrim);
  ctx.font = `700 11px ${fontFamilies.mono}`;
  ctx.textAlign = 'left';
  ctx.textBaseline = 'top';
  ctx.fillText(
    record ? `PB  ${formatRunTime(record.bestTimeMs)}s` : 'PB  --.--s',
    24,
    20,
  );
  ctx.fillStyle = c(statusTones.warning.dot);
  ctx.font = `700 10px ${fontFamilies.mono}`;
  ctx.fillText(
    `★★★ <= ${formatRunTime(state.level.stars.threeStarMs)}s`,
    24,
    36,
  );
  if (showTries) {
    ctx.save();
    ctx.globalAlpha = 0.72;
    ctx.fillStyle = c(fleetColors.onScrim);
    ctx.font = `600 9.5px ${fontFamilies.mono}`;
    ctx.fillText(`ATTEMPT #${state.attemptCount}`, 24, 51);
    ctx.restore();
  }

  // Top-right: broken devices remaining pill
  const remaining = state.devices.filter((d) => !d.repaired).length;
  const total = state.devices.length;
  drawDarkHudBox(
    ctx,
    c,
    WORLD_WIDTH - 172,
    12,
    158,
    42,
    10,
    remaining === 0 ? statusTones.success.dot : fleetColors.scrimHover,
  );

  ctx.fillStyle = c(
    remaining === 0 ? statusTones.success.dot : fleetColors.onScrim,
  );
  ctx.font = `800 13px ${fontFamilies.mono}`;
  ctx.textAlign = 'center';
  ctx.textBaseline = 'middle';
  ctx.fillText(
    remaining === 0 ? 'FLEET HEALTHY!' : `FIXED ${total - remaining}/${total}`,
    WORLD_WIDTH - 93,
    33,
  );

  // Brief crimson flash on hazard reset
  if (state.hazardFlashTimer > 0) {
    ctx.save();
    ctx.globalAlpha = Math.min(0.3, (state.hazardFlashTimer / 0.24) * 0.3);
    ctx.fillStyle = c(statusTones.error.dot);
    ctx.fillRect(0, 0, WORLD_WIDTH, WORLD_HEIGHT);
    ctx.restore();
  }

  ctx.restore();
}

export function renderGame(
  ctx: CanvasRenderingContext2D,
  state: GameState,
  assets: RendererAssets,
  record: StageRecord | undefined,
  nowMs: number,
): void {
  const c = createColorResolver(ctx);
  const nowSec = nowMs / 1000;
  ctx.save();

  if (state.shakeTimer > 0 && state.shakeIntensity > 0) {
    const ox = (Math.random() - 0.5) * 2 * state.shakeIntensity;
    const oy = (Math.random() - 0.5) * 2 * state.shakeIntensity;
    ctx.translate(ox, oy);
  }

  drawBackground(ctx, c, state);

  // Subtle world hints
  ctx.save();
  ctx.font = `600 11px ${fontFamilies.text}`;
  ctx.textAlign = 'center';
  ctx.textBaseline = 'middle';
  for (const hint of state.level.hints) {
    const bw = ctx.measureText(hint.text).width + 20;
    const bh = 22;
    ctx.save();
    ctx.globalAlpha = 0.9;
    ctx.fillStyle = c(fleetColors.surface);
    drawRoundedRect(ctx, hint.x - bw / 2, hint.y - bh / 2, bw, bh, 11);
    ctx.fill();
    ctx.restore();
    ctx.strokeStyle = c(fleetColors.borderPrimary);
    ctx.lineWidth = 1;
    drawRoundedRect(ctx, hint.x - bw / 2, hint.y - bh / 2, bw, bh, 11);
    ctx.stroke();
    ctx.fillStyle = c(fleetColors.textPrimary);
    ctx.fillText(hint.text, hint.x, hint.y + 0.5);
  }
  ctx.restore();

  drawPlatformsAndHazards(ctx, c, state, nowSec);

  for (const dev of state.devices) {
    drawDevice(ctx, c, dev, nowSec);
  }

  if (record && record.ghost.length > 0 && state.clockStarted) {
    drawGhost(ctx, c, assets, record.ghost, state.elapsedMs, nowSec);
  }

  for (const sw of state.shockwaves) {
    ctx.save();
    ctx.globalAlpha = Math.max(0, sw.life / sw.maxLife);
    ctx.strokeStyle = c(sw.color);
    ctx.lineWidth = 3.5;
    ctx.beginPath();
    ctx.arc(sw.x, sw.y, sw.radius, 0, Math.PI * 2);
    ctx.stroke();
    ctx.restore();
  }

  const p = state.player;
  const tilt = Math.max(-0.24, Math.min(0.24, p.vx / 1800));
  drawNinjaBass(
    ctx,
    c,
    assets,
    p.x,
    p.y,
    p.w,
    p.h,
    p.facing,
    state.phase === 'cleared',
    p.scaleX,
    p.scaleY,
    tilt,
    nowSec,
  );

  drawSlashArc(ctx, c, state);

  for (const star of state.shurikens) {
    ctx.save();
    ctx.globalAlpha = 0.36;
    ctx.strokeStyle = c(fleetColors.primary);
    ctx.lineWidth = 5;
    ctx.lineCap = 'round';
    ctx.beginPath();
    ctx.moveTo(star.x, star.y);
    ctx.lineTo(star.x - Math.sign(star.vx) * 28, star.y);
    ctx.stroke();
    ctx.restore();

    drawAutoRepairGear(
      ctx,
      c,
      star.x,
      star.y,
      10,
      star.rotation,
      fleetColors.primary,
      fleetColors.textPrimary,
    );
  }

  for (const pt of state.particles) {
    ctx.save();
    ctx.globalAlpha = Math.max(0, pt.life / pt.maxLife);
    ctx.fillStyle = c(pt.color);
    ctx.fillRect(pt.x - pt.size / 2, pt.y - pt.size / 2, pt.size, pt.size);
    ctx.restore();
  }

  for (const callout of state.callouts) {
    ctx.save();
    ctx.globalAlpha = Math.min(1, (callout.life / callout.maxLife) * 1.4);
    ctx.font = `900 12px ${fontFamilies.mono}`;
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.lineWidth = 3;
    ctx.strokeStyle = c(fleetColors.surface);
    ctx.strokeText(callout.text, callout.x, callout.y);
    ctx.fillStyle = c(callout.color);
    ctx.fillText(callout.text, callout.x, callout.y);
    ctx.restore();
  }

  ctx.restore();

  drawHud(ctx, c, state, record, nowSec);
}
