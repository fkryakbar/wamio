export interface VerticalRect {
  top: number;
  bottom: number;
  height: number;
}

// Tall bubbles (long text, images, documents) may never have 60% of their
// height inside the viewport. A small, fixed visible slice is sufficient to
// treat them as seen, while short bubbles still use the proportional rule.
export function isMeaningfullyVisible(
  rect: VerticalRect,
  bounds: Pick<VerticalRect, 'top' | 'bottom'>,
): boolean {
  const intersection = Math.max(
    0,
    Math.min(rect.bottom, bounds.bottom) - Math.max(rect.top, bounds.top),
  );
  return rect.height > 0 && intersection >= Math.min(rect.height * 0.6, 48);
}
