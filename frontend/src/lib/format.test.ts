import { describe, it, expect, vi, afterEach } from 'vitest';
import { relativeTime, number } from './format';
describe('format', () => {
  afterEach(() => vi.useRealTimers());
  it('formats relative time and numbers', () => {
    vi.useFakeTimers(); vi.setSystemTime(new Date('2026-01-01T01:00:00Z'));
    expect(relativeTime('2026-01-01T00:00:00Z')).toBe('1h ago'); expect(number(12345)).toBe('12,345');
  });
});
