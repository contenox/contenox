import { DetailsPanel, Panel } from '@contenox/ui';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { CHAIN_HANDLER_OPTIONS } from '../../../../../lib/chainHandlerOptions';
import { ChainTask } from '../../../../../lib/types';

interface TaskDetailsPanelProps {
  task: ChainTask;
  onClose: () => void;
  onSave: (task: ChainTask) => void;
  onDelete?: (taskId: string) => void;
  isNewTask?: boolean;
}

interface ExecuteConfigDisplay {
  provider?: string;
  model?: string;
  temperature?: number;
  max_tokens?: number;
}

const TaskDetailsPanel: React.FC<TaskDetailsPanelProps> = ({
  task,
  onClose,
  onSave,
  onDelete,
  isNewTask = false,
}) => {
  const { t } = useTranslation();
  const [isEditing, setIsEditing] = useState(isNewTask);
  const [editedTask, setEditedTask] = useState<ChainTask>({ ...task });

  // Update edited task when task prop changes
  useEffect(() => {
    setEditedTask({ ...task });
  }, [task]);

  const handlerOptions = CHAIN_HANDLER_OPTIONS.map(option => ({
    value: option.value,
    label: option.label,
  }));

  const fields = [
    {
      key: 'id',
      label: t('chains.task_id'),
      type: 'text' as const,
    },
    {
      key: 'handler',
      label: t('workflow.task_type'),
      type: 'select' as const,
      options: handlerOptions,
    },
    {
      key: 'description',
      label: t('workflow.description'),
      type: 'text' as const,
    },
    {
      key: 'prompt_template',
      label: t('workflow.prompt_template'),
      type: 'textarea' as const,
    },
    {
      key: 'input_var',
      label: t('workflow.input_variable'),
      type: 'text' as const,
    },
    {
      key: 'system_instruction',
      label: t('chains.system_instruction'),
      type: 'textarea' as const,
    },
    {
      key: 'timeout',
      label: t('workflow.timeout'),
      type: 'text' as const,
    },
    {
      key: 'retry_on_failure',
      label: t('workflow.retry_on_failure'),
      type: 'text' as const,
    },
  ];

  // Custom render for complex nested data
  const renderTransitionData = (transition: ChainTask['transition']) => {
    return (
      <div className="space-y-3">
        <div>
          <strong>{t('workflow.on_failure')}:</strong> {transition.on_failure || 'None'}
        </div>
        <div>
          <strong>{t('workflow.branches')}:</strong>
          <div className="space-y-2 pt-2">
            {transition.branches.map((branch, index) => (
              <Panel key={index} variant="body" className="pl-3">
                <div>
                  <strong>Condition:</strong> {branch.when || 'Default'}
                </div>
                <div>
                  <strong>Go to:</strong> {branch.goto}
                </div>
                <div>
                  <strong>Operator:</strong> {branch.operator}
                </div>
                {branch.compose && (
                  <div className="text-xs pt-1">
                    <div>
                      <strong>compose.with_var:</strong> {branch.compose.with_var || '(none)'}
                    </div>
                    <div>
                      <strong>compose.strategy:</strong> {branch.compose.strategy || '(default)'}
                    </div>
                  </div>
                )}
              </Panel>
            ))}
          </div>
        </div>
      </div>
    );
  };

  const renderExecuteConfig = (value: ExecuteConfigDisplay | null | undefined) => (
    <div className="space-y-2">
      {value?.provider && (
        <div>
          <strong>Provider:</strong> {value.provider}
        </div>
      )}
      {value?.model && (
        <div>
          <strong>Model:</strong> {value.model}
        </div>
      )}
      {value?.temperature && (
        <div>
          <strong>Temperature:</strong> {value.temperature}
        </div>
      )}
      {value?.max_tokens && (
        <div>
          <strong>Max Tokens:</strong> {value.max_tokens}
        </div>
      )}
    </div>
  );

  const extendedFields = [
    ...fields,
    {
      key: 'transition',
      label: t('workflow.transitions'),
      type: 'custom' as const,
      render: (value: unknown) => renderTransitionData(value as ChainTask['transition']),
    },
    {
      key: 'execute_config',
      label: t('chains.execute_configuration'),
      type: 'custom' as const,
      render: (value: unknown) => renderExecuteConfig(value as ExecuteConfigDisplay | null | undefined),
    },
  ];

  const handleEditToggle = (editing: boolean) => {
    setIsEditing(editing);
    if (!editing && !isNewTask) {
      setEditedTask({ ...task });
    }
  };

  const handleFieldUpdate = (updates: Record<string, unknown>) => {
    setEditedTask(prev => ({
      ...prev,
      ...(updates as Partial<ChainTask>),
    }));
  };

  const handleSave = (data: Record<string, unknown>) => {
    // Convert the generic data back to ChainTask
    const d = data as Partial<ChainTask>;
    const updatedTask: ChainTask = {
      id: d.id || editedTask.id,
      description: d.description || editedTask.description,
      handler: d.handler || editedTask.handler,
      prompt_template: d.prompt_template || editedTask.prompt_template,
      transition: d.transition || editedTask.transition,
      system_instruction: d.system_instruction,
      execute_config: d.execute_config,
      print: d.print,
      output_template: d.output_template,
      input_var: d.input_var,
      timeout: d.timeout,
      retry_on_failure: d.retry_on_failure,
    };

    onSave(updatedTask);
    if (!isNewTask) {
      setIsEditing(false);
    }
  };

  return (
    <DetailsPanel
      title={editedTask.id}
      data={editedTask as unknown as Record<string, unknown>}
      fields={extendedFields}
      onClose={onClose}
      onSave={handleSave}
      onDelete={onDelete ? () => onDelete(editedTask.id) : undefined}
      isEditing={isEditing}
      onEditToggle={handleEditToggle}
      onFieldUpdate={handleFieldUpdate}
    />
  );
};

export default TaskDetailsPanel;
