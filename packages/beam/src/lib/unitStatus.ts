import type { Status } from '@contenox/ui';
import type { TranslationKey } from '../i18n';
import type { FleetInstanceState, MissionReport, MissionStatus } from './types';

/**
 * The composed status of ONE fleet unit, rendered identically wherever a unit
 * or a mission appears — the fleet board, the mission list, the mission detail,
 * and the inbox. It exists because the same unit used to read as a
 * contradiction across surfaces: the board showed the instance's *process*
 * state ("running", green) while the mission pages showed the mission's *status*
 * ("open") plus its heartbeat ("never reported") — three different truths about
 * one unit with nothing tying them together.
 *
 * The fix is not to pick one truth but to render them together, each labelled
 * for what it actually reports, so they stop looking like they disagree:
 *
 *   - blocked  — the loudest fact. The unit's latest report is a `blocker`: it
 *     is waiting on a human. Rendered first and prominently.
 *   - process  — is the agent process up? (instance state: running / starting /
 *     stopped / warning / error). The "is it alive" truth.
 *   - verdict  — has the mission produced an outcome? (mission status). Crucially
 *     `open` reads as "no result yet", NEVER as a health light — an open mission
 *     is not a warning, it simply has not landed.
 *   - liveness — when did the unit last say it was alive? (heartbeat age, or an
 *     honest "no heartbeat" when it has never reported — a neutral fact, not an
 *     alarm).
 *
 * `composeUnitStatus` is pure and total: it takes whatever facts a caller has
 * (each optional) and returns the ordered atoms to render. A caller with only
 * an instance state (a running unit not bound to a mission) gets one atom; the
 * mission list, which joins the fleet, gets the full picture.
 */

export type UnitStatusFacts = {
  /** Process truth: the live agent instance's lifecycle state, when known. */
  instanceState?: FleetInstanceState;
  /** Verdict truth: the mission's status, when this unit is on a mission. */
  missionStatus?: MissionStatus;
  /** Liveness: the mission's last heartbeat (ISO), when it has ever reported. */
  lastHeartbeat?: string;
  /** Whether the unit's latest report is a `blocker` — it is waiting on a human. */
  blocked?: boolean;
};

export type UnitStatusBadgeVariant = 'success' | 'error' | 'warning' | 'outline' | 'secondary';

export type UnitStatusAtomKind = 'blocked' | 'process' | 'verdict' | 'liveness';

export type UnitStatusAtom = {
  kind: UnitStatusAtomKind;
  /** A static label key (process/verdict/blocked, and liveness when never reported). */
  labelKey?: TranslationKey;
  /** A tooltip key explaining what this fragment reports. */
  titleKey?: TranslationKey;
  /** Process only: the StatusIndicator dot to render. */
  indicator?: Status;
  /** Everything but process: the Badge variant to render. */
  variant?: UnitStatusBadgeVariant;
  /** Liveness only, when present: an ISO timestamp to render as relative time. */
  heartbeat?: string;
};

/**
 * Instance state → StatusIndicator dot. Identical to the mapping the fleet board
 * has always used (running → green ✓, starting → amber ⟳, stopped → neutral ○,
 * warning/error → their alert colours) — hoisted here so the board and every
 * other surface read the process dot the same way.
 */
export const PROCESS_STATUS_INDICATOR: Record<FleetInstanceState, Status> = {
  running: 'completed',
  starting: 'in-progress',
  stopped: 'planned',
  warning: 'warning',
  error: 'error',
};

export const PROCESS_LABEL_KEY: Record<FleetInstanceState, TranslationKey> = {
  starting: 'fleet.state.starting',
  running: 'fleet.state.running',
  stopped: 'fleet.state.stopped',
  warning: 'fleet.state.warning',
  error: 'fleet.state.error',
};

// Mission status → verdict chip. `open` is deliberately an outline (neutral)
// badge labelled "no result yet": an open mission has produced no outcome, which
// is not the same as — and must never be coloured like — a health warning.
const VERDICT: Record<
  MissionStatus,
  { labelKey: TranslationKey; titleKey: TranslationKey; variant: UnitStatusBadgeVariant }
> = {
  open: { labelKey: 'unit.verdict_open', titleKey: 'unit.verdict_open_title', variant: 'outline' },
  landed: {
    labelKey: 'unit.verdict_landed',
    titleKey: 'unit.verdict_landed_title',
    variant: 'success',
  },
  derailed: {
    labelKey: 'unit.verdict_derailed',
    titleKey: 'unit.verdict_derailed_title',
    variant: 'error',
  },
  abandoned: {
    labelKey: 'unit.verdict_abandoned',
    titleKey: 'unit.verdict_abandoned_title',
    variant: 'secondary',
  },
};

/** The ordered status atoms to render for a unit — loudest (blocked) first. */
export function composeUnitStatus(facts: UnitStatusFacts): UnitStatusAtom[] {
  const atoms: UnitStatusAtom[] = [];

  if (facts.blocked) {
    atoms.push({
      kind: 'blocked',
      labelKey: 'unit.blocked',
      titleKey: 'unit.blocked_title',
      variant: 'warning',
    });
  }

  if (facts.instanceState) {
    atoms.push({
      kind: 'process',
      indicator: PROCESS_STATUS_INDICATOR[facts.instanceState],
      labelKey: PROCESS_LABEL_KEY[facts.instanceState],
    });
  }

  if (facts.missionStatus) {
    const v = VERDICT[facts.missionStatus];
    atoms.push({ kind: 'verdict', labelKey: v.labelKey, titleKey: v.titleKey, variant: v.variant });
  }

  // Liveness is a mission concept (heartbeat), so it only shows for a unit on a
  // mission. A never-reported unit renders honestly rather than blank — a
  // neutral "no heartbeat", not a warning.
  if (facts.missionStatus) {
    atoms.push(
      facts.lastHeartbeat
        ? {
            kind: 'liveness',
            heartbeat: facts.lastHeartbeat,
            titleKey: 'unit.liveness_title',
            variant: 'secondary',
          }
        : {
            kind: 'liveness',
            labelKey: 'missions.heartbeat_never',
            titleKey: 'unit.liveness_never_title',
            variant: 'secondary',
          },
    );
  }

  return atoms;
}

// An unparseable/absent timestamp sorts oldest rather than poisoning the sort.
function sortableTime(iso: string): number {
  const parsed = Date.parse(iso);
  return Number.isNaN(parsed) ? -Infinity : parsed;
}

/**
 * The set of mission ids whose NEWEST report is a `blocker` — i.e. missions
 * currently waiting on a human. Fed the flat report feed (any missions), it
 * keeps only the latest report per mission and returns those blocked, so a
 * mission that hit a blocker and then filed a later progress/result report is
 * correctly no longer counted as blocked.
 */
export function blockedMissionIds(reports: MissionReport[]): Set<string> {
  const newest = new Map<string, MissionReport>();
  for (const report of reports) {
    const current = newest.get(report.missionId);
    if (!current || sortableTime(report.createdAt) > sortableTime(current.createdAt)) {
      newest.set(report.missionId, report);
    }
  }
  const blocked = new Set<string>();
  for (const [missionId, report] of newest) {
    if (report.kind === 'blocker') blocked.add(missionId);
  }
  return blocked;
}
