import { describe, expect, it } from 'vitest';
import {
  heartbeatLabel,
  MISSION_STATUS_INDICATOR,
  MISSION_STATUS_LABEL_KEY,
  REPORT_KIND_BADGE_VARIANT,
  REPORT_KIND_LABEL_KEY,
} from './missionPresentation';

describe('heartbeatLabel', () => {
  it('is honest about never having reported — a distinct label, not a blank or a raw fallback', () => {
    const label = heartbeatLabel(undefined, 'en', { never: 'Never reported', justNow: 'just now' });
    expect(label).toBe('Never reported');
  });

  it('renders a real heartbeat as relative time, never the "never" label', () => {
    const fiveMinAgo = new Date(Date.now() - 5 * 60 * 1000).toISOString();
    const label = heartbeatLabel(fiveMinAgo, 'en', {
      never: 'Never reported',
      justNow: 'just now',
    });
    expect(label).not.toBe('Never reported');
    expect(label).toContain('minute');
  });

  it('a heartbeat from moments ago reads as "just now", still distinct from "never"', () => {
    const label = heartbeatLabel(new Date().toISOString(), 'en', {
      never: 'Never reported',
      justNow: 'just now',
    });
    expect(label).toBe('just now');
  });
});

describe('mission status -> StatusIndicator status', () => {
  it('covers every MissionStatus with a distinct, honest indicator', () => {
    expect(MISSION_STATUS_INDICATOR.open).toBe('in-progress');
    expect(MISSION_STATUS_INDICATOR.landed).toBe('completed');
    expect(MISSION_STATUS_INDICATOR.derailed).toBe('error');
    expect(MISSION_STATUS_INDICATOR.abandoned).toBe('planned');
  });

  it('gives every status its own translation key', () => {
    const keys = Object.values(MISSION_STATUS_LABEL_KEY);
    expect(new Set(keys).size).toBe(keys.length);
  });
});

describe('report kind -> badge variant', () => {
  it('gives blocker an alarming variant, distinct from progress — the M2 requirement', () => {
    expect(REPORT_KIND_BADGE_VARIANT.blocker).toBe('error');
    expect(REPORT_KIND_BADGE_VARIANT.progress).not.toBe(REPORT_KIND_BADGE_VARIANT.blocker);
  });

  it('gives every kind a distinct variant so none is visually confusable with another', () => {
    const variants = Object.values(REPORT_KIND_BADGE_VARIANT);
    expect(new Set(variants).size).toBe(variants.length);
  });

  it('gives every kind its own translation key', () => {
    const keys = Object.values(REPORT_KIND_LABEL_KEY);
    expect(new Set(keys).size).toBe(keys.length);
  });
});
