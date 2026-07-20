import type { Status } from '@contenox/ui';
import type { TranslationKey } from '../../../i18n';
import { relativeTime } from '../../../lib/relativeTime';
import type { MissionReportKind, MissionStatus } from '../../../lib/types';

/**
 * Mission status → StatusIndicator status, mirroring FleetPage's
 * STATE_STATUS idiom. `open` reads as in-progress (still running, amber ⟳);
 * `landed` as completed (green ✓); `derailed` as error (red ✗); `abandoned`
 * as planned (neutral ○) — closer to "declared but not running" than to
 * either a success or a failure, the same reasoning FleetPage uses for a
 * `stopped` instance.
 */
export const MISSION_STATUS_INDICATOR: Record<MissionStatus, Status> = {
  open: 'in-progress',
  landed: 'completed',
  derailed: 'error',
  abandoned: 'planned',
};

export const MISSION_STATUS_LABEL_KEY: Record<MissionStatus, TranslationKey> = {
  open: 'missions.status.open',
  landed: 'missions.status.landed',
  derailed: 'missions.status.derailed',
  abandoned: 'missions.status.abandoned',
};

/**
 * A Badge variant subset strong enough that `blocker` cannot be mistaken for
 * `progress` at a glance — the M2 requirement this map exists to satisfy.
 */
type ReportBadgeVariant = 'error' | 'accent' | 'secondary' | 'success';

export const REPORT_KIND_BADGE_VARIANT: Record<MissionReportKind, ReportBadgeVariant> = {
  blocker: 'error',
  finding: 'accent',
  progress: 'secondary',
  result: 'success',
};

export const REPORT_KIND_LABEL_KEY: Record<MissionReportKind, TranslationKey> = {
  progress: 'missions.report_kind.progress',
  finding: 'missions.report_kind.finding',
  blocker: 'missions.report_kind.blocker',
  result: 'missions.report_kind.result',
};

/**
 * Renders a mission's LastHeartbeat honestly: `undefined` ("never reported" —
 * missionservice.Heartbeat has no caller yet, see the Mission type's doc
 * comment) is a DIFFERENT fact from "reported a long time ago", so it gets
 * its own label instead of falling through relativeTime, where an unparseable
 * empty string would otherwise echo back as a blank or a raw "".
 */
export function heartbeatLabel(
  lastHeartbeat: string | undefined,
  locale: string,
  labels: { never: string; justNow: string },
): string {
  if (!lastHeartbeat) return labels.never;
  return relativeTime(lastHeartbeat, locale, labels.justNow);
}
