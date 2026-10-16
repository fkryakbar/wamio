import { describe, expect, it } from 'vitest';
import { isMeaningfullyVisible } from './messageVisibility';

describe('isMeaningfullyVisible', () => {
  const viewport = { top: 100, bottom: 500 };

  it('marks a short bubble read only after most of it is visible', () => {
    expect(isMeaningfullyVisible({ top: 470, bottom: 520, height: 50 }, viewport)).toBe(true);
    expect(isMeaningfullyVisible({ top: 480, bottom: 530, height: 50 }, viewport)).toBe(false);
  });

  it('allows a fixed visible slice for a message taller than the viewport', () => {
    expect(isMeaningfullyVisible({ top: 450, bottom: 1450, height: 1000 }, viewport)).toBe(true);
  });

  it('rejects messages outside the viewport', () => {
    expect(isMeaningfullyVisible({ top: 510, bottom: 610, height: 100 }, viewport)).toBe(false);
  });
});
