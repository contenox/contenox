import { Panel, Textarea, Label, P } from '@contenox/ui';
import { useTranslation } from 'react-i18next';
import {
  FormTask,
  HandleChatCompletion,
  HandleExecuteToolCalls,
  HandleNoop,
  HandleRaiseError,
  HandleRoute,
  HandleTools,
} from '../../../../../../lib/types';
import LLMConfigFields from './LLMConfigFields';

interface HandlerSpecificFieldsProps {
  task: FormTask;
  onChange: (updates: Partial<FormTask>) => void;
}

export default function HandlerSpecificFields({ task, onChange }: HandlerSpecificFieldsProps) {
  const { t } = useTranslation();

  const renderPromptBlock = (placeholder: string) => (
    <div>
      <Label className="block text-sm font-medium">{t('chains.task_form.prompt_template')}</Label>
      <Textarea
        rows={4}
        value={task.prompt_template || ''}
        onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) =>
          onChange({ prompt_template: e.target.value })
        }
        placeholder={placeholder}
      />
      {/*{help ? <p className="text-text-muted mt-1 text-xs">{t(help)}</p> : null}*/}
    </div>
  );

  // LLM-like
  if (task.handler === HandleChatCompletion || task.handler === HandleExecuteToolCalls) {
    return (
      <Panel variant="surface" className="p-4">
        <LLMConfigFields task={task} onChange={onChange} expanded />
      </Panel>
    );
  }

  if (task.handler === HandleRoute) {
    return (
      <Panel variant="surface" className="space-y-4 p-4">
        {renderPromptBlock('Ask for one of the transition labels...')}
        <LLMConfigFields task={task} onChange={onChange} />
      </Panel>
    );
  }

  // raise_error
  if (task.handler === HandleRaiseError) {
    return (
      <Panel variant="surface" className="space-y-2 p-4">
        <Label className="block text-sm font-medium">{t('chains.task_form.error_message')}</Label>
        <Textarea
          rows={3}
          value={task.prompt_template || ''}
          onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) =>
            onChange({ prompt_template: e.target.value })
          }
          placeholder="Error: Validation failed because..."
        />
        <P className="text-text-muted mt-1 text-xs">{t('chains.task_form.error_message_help')}</P>
      </Panel>
    );
  }

  // noop / default
  if (task.handler === HandleNoop) {
    return (
      <Panel variant="surface" className="p-4">
        <div className="text-text-muted text-sm">{t('chains.task_form.noop_description')}</div>
      </Panel>
    );
  }

  if (task.handler === HandleTools) {
    return (
      <Panel variant="surface" className="p-4">
        <div className="text-text-muted text-sm">
          {t('chains.task_form.tools_json_description', 'Configure direct tools tasks in the JSON tab.')}
        </div>
      </Panel>
    );
  }

  return (
    <Panel variant="surface" className="p-4">
      <div className="text-text-muted text-sm">{t('chains.task_form.no_additional_config')}</div>
    </Panel>
  );
}
