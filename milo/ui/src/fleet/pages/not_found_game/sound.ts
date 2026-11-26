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

class TenSecondBassSound {
  private ctx: AudioContext | null = null;
  private muted = false;

  public isMuted(): boolean {
    return this.muted;
  }

  public toggleMute(): boolean {
    this.muted = !this.muted;
    return this.muted;
  }

  private ensureContext(): AudioContext | null {
    if (this.muted || typeof window === 'undefined') {
      return null;
    }
    if (!this.ctx) {
      const AudioCtor = window.AudioContext;
      if (!AudioCtor) return null;
      try {
        this.ctx = new AudioCtor();
      } catch {
        return null;
      }
    }
    if (this.ctx.state === 'suspended') {
      this.ctx.resume().catch(() => {
        // Ignore resume blocks until next user gesture.
      });
    }
    return this.ctx;
  }

  private playTone(
    freqStart: number,
    freqEnd: number,
    durationSec: number,
    type: OscillatorType,
    gainPeak: number,
    delaySec = 0,
  ): void {
    const ctx = this.ensureContext();
    if (!ctx) return;

    const now = ctx.currentTime + delaySec;
    const osc = ctx.createOscillator();
    const gain = ctx.createGain();

    osc.type = type;
    osc.frequency.setValueAtTime(freqStart, now);
    osc.frequency.exponentialRampToValueAtTime(
      Math.max(20, freqEnd),
      now + durationSec,
    );

    gain.gain.setValueAtTime(0.0001, now);
    gain.gain.linearRampToValueAtTime(gainPeak, now + 0.008);
    gain.gain.exponentialRampToValueAtTime(0.0001, now + durationSec);

    osc.connect(gain);
    gain.connect(ctx.destination);
    osc.start(now);
    osc.stop(now + durationSec + 0.02);
  }

  public playJump(variant: 'ground' | 'double' | 'wall'): void {
    if (variant === 'double') {
      this.playTone(380, 760, 0.085, 'triangle', 0.09);
    } else if (variant === 'wall') {
      this.playTone(320, 640, 0.085, 'triangle', 0.09);
    } else {
      this.playTone(250, 490, 0.08, 'triangle', 0.085);
    }
  }

  public playSlash(): void {
    this.playTone(860, 210, 0.075, 'sawtooth', 0.075);
    this.playTone(1360, 480, 0.06, 'triangle', 0.05, 0.01);
  }

  public playShuriken(): void {
    this.playTone(1180, 640, 0.07, 'square', 0.045);
    this.playTone(1760, 980, 0.055, 'sine', 0.04, 0.015);
  }

  public playShurikenEmpty(): void {
    // Crisp dry click when attempting to throw with 0 stars remaining
    this.playTone(210, 140, 0.04, 'square', 0.045);
  }

  public playShieldDeflect(): void {
    this.playTone(1850, 2450, 0.06, 'triangle', 0.085);
    this.playTone(1380, 920, 0.09, 'square', 0.045, 0.02);
  }

  public playRepair(combo: number): void {
    const base = 520 + Math.min(combo, 7) * 75;
    this.playTone(base, base * 1.45, 0.1, 'sine', 0.11);
    this.playTone(base * 1.5, base * 2, 0.11, 'triangle', 0.06, 0.035);
  }

  public playCountdownTick(): void {
    this.playTone(880, 660, 0.05, 'triangle', 0.075);
  }

  public playStageClear(): void {
    const notes = [523.25, 659.25, 783.99, 1046.5, 1318.5];
    notes.forEach((n, idx) => {
      this.playTone(n, n * 1.01, 0.22, 'triangle', 0.095, idx * 0.05);
    });
  }

  public playTimeUp(): void {
    this.playTone(340, 220, 0.14, 'sawtooth', 0.08, 0);
    this.playTone(220, 130, 0.22, 'sawtooth', 0.08, 0.12);
  }

  public playReset(): void {
    this.playTone(320, 175, 0.075, 'sine', 0.06);
  }
}

export const tenSecondBassSound = new TenSecondBassSound();
