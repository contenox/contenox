import { Badge, Span } from '@contenox/ui';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import { useMissions } from '../hooks/useMissions';

/**
 * The header strip for an ADOPTED chat session — one the operator opened from
 * the fleet board / mission detail to watch and steer a running dispatch (see
 * `adoptMeta.ts`). Mounted only when the session's roster `_meta` carries a
 * `contenox.adopt` outcome, so its `useMissions` poll is scoped to adopted
 * sessions and never runs for an ordinary chat.
 *
 * It reports the three things that make an adopted session legible:
 *  - control: "Übernommen" (this connection answers the unit's permission asks)
 *    vs "Beobachten" (another viewer controls it; you are watching) — read
 *    straight from the kernel's `controller` verdict, never assumed.
 *  - the agent driving the session.
 *  - a link back to the mission it belongs to, resolved from the adopted
 *    instance id (a dispatched mission carries its instance id); absent for an
 *    adopted board session that is not a mission.
 *
 * plus the durability caveat: dispatch wrote nothing durable before adoption, so
 * the transcript is durable only from take-over onward (acpsvc/adopt.go "Honest
 * limitations"). Shown quietly so it informs without alarming.
 */
export function AdoptedSessionBanner({
  instanceId,
  controller,
  agentName,
}: {
  instanceId: string;
  controller: boolean;
  agentName: string | null;
}) {
  const { t } = useTranslation();
  const { data: missions } = useMissions();
  const mission = (missions ?? []).find(m => m.instanceId === instanceId);

  return (
    <div className="border-surface-200 dark:border-dark-surface-600 bg-surface-50 dark:bg-dark-surface-200 shrink-0 border-b px-3 py-2 sm:px-4">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <Badge
          variant={controller ? 'success' : 'secondary'}
          size="sm"
          title={
            controller
              ? t('acp_chat.adopted_controller_title')
              : t('acp_chat.adopted_observer_title')
          }>
          {controller ? t('acp_chat.adopted_controller_label') : t('acp_chat.adopted_observer_label')}
        </Badge>
        {agentName && (
          <Span className="text-text-muted dark:text-dark-text-muted text-xs">
            {t('acp_chat.agent_label', { name: agentName })}
          </Span>
        )}
        {mission && (
          <Link
            to={`/missions/${mission.id}`}
            className="text-primary dark:text-dark-primary text-xs hover:underline"
            title={mission.intent}>
            {t('acp_chat.adopted_mission_link')}
          </Link>
        )}
      </div>
      <p className="text-text-muted dark:text-dark-text-muted mt-1 text-xs">
        {t('acp_chat.adopted_durability_notice')}
      </p>
    </div>
  );
}
