import { Badge, Card, P, Panel } from '@contenox/ui';
import { useTranslation } from 'react-i18next';
import { LocalHook } from '../../../../lib/types';

type LocalHookCardProps = {
  hook: LocalHook;
};

export default function LocalHookCard({ hook }: LocalHookCardProps) {
  const { t } = useTranslation();

  // Safe access to tools with fallback
  const tools = hook.tools || [];

  const sourceLabel = (() => {
    if (hook.source === 'mcp') return t('local_hooks.source_mcp');
    if (hook.source === 'remote') return t('local_hooks.source_remote');
    if (hook.source === 'builtin') return t('local_hooks.source_builtin');
    return hook.type;
  })();

  return (
    <Card variant="surface" className="h-full">
      <div className="flex h-full flex-col gap-3">
        {/* Header */}
        <div className="flex items-start justify-between">
          <div className="flex-1">
            <P className="font-semibold">{hook.name}</P>
            <P className="text-muted text-sm">{hook.description}</P>
          </div>
          <Badge variant="default" size="sm">
            {sourceLabel}
          </Badge>
        </div>

        {/* Tools */}
        <div className="flex-1">
          {hook.unavailableReason ? (
            <Panel variant="warning" className="text-sm">
              <P className="font-medium">{t('local_hooks.unavailable_title')}</P>
              <P className="text-muted font-mono text-xs break-words">{hook.unavailableReason}</P>
            </Panel>
          ) : (
            <>
              <P className="text-muted text-sm font-medium">
                {t('local_hooks.tools')} ({tools.length})
              </P>
              <div className="space-y-2">
                {tools.map((tool, index) => (
                  <div key={index} className="bg-secondary/20 rounded px-3 py-2">
                    <P className="text-sm font-medium">{tool.function?.name || 'Unnamed Tool'}</P>
                    <P className="text-muted line-clamp-2 text-xs">
                      {tool.function?.description || 'No description available'}
                    </P>
                    {tool.function?.parameters && (
                      <P className="text-muted mt-1 text-xs">
                        {t('local_hooks.parameters_defined')}
                      </P>
                    )}
                  </div>
                ))}
                {tools.length === 0 && (
                  <P className="text-muted text-sm italic">{t('local_hooks.no_tools_available')}</P>
                )}
              </div>
            </>
          )}
        </div>
      </div>
    </Card>
  );
}
