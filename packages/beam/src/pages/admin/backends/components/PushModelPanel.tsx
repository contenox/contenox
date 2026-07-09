import { Button, Input, InlineNotice, Label, Spinner } from '@contenox/ui';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePushModel } from '../../../../hooks/useBackends';
import { formatBytes } from '../../../../lib/format';

type PushModelPanelProps = {
  backendId: string;
};

// Deploys a local GGUF file to this backend's modeld node (local or remote) —
// the UI twin of `contenox model push`. Collapsed by default: this is an
// occasional admin action, not something every backend card should always show.
export function PushModelPanel({ backendId }: PushModelPanelProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState('');
  const [file, setFile] = useState<File | null>(null);
  const pushModel = usePushModel(backendId);

  const canPush = name.trim() && file && !pushModel.isPending;

  const handlePush = () => {
    if (!file || !name.trim()) return;
    pushModel.mutate(
      { name: name.trim(), file },
      {
        onSuccess: () => {
          setFile(null);
        },
      },
    );
  };

  if (!open) {
    return (
      <Button type="button" variant="secondary" size="sm" onClick={() => setOpen(true)}>
        {t('backends.push_toggle')}
      </Button>
    );
  }

  return (
    <div className="space-y-2 rounded-md border border-border p-3">
      <Label className="block text-sm font-medium">{t('backends.push_name_label')}</Label>
      <Input
        value={name}
        onChange={e => setName(e.target.value)}
        placeholder={t('backends.push_name_placeholder')}
        disabled={pushModel.isPending}
      />
      <Label className="block text-sm font-medium">{t('backends.push_file_label')}</Label>
      <input
        type="file"
        accept=".gguf"
        onChange={e => setFile(e.target.files?.[0] ?? null)}
        disabled={pushModel.isPending}
        className="block w-full text-sm"
      />
      <div className="flex items-center gap-2">
        <Button type="button" variant="primary" size="sm" disabled={!canPush} onClick={handlePush}>
          {pushModel.isPending ? (
            <>
              <Spinner size="sm" className="mr-2" />
              {t('common.uploading')}
            </>
          ) : (
            t('backends.push_action')
          )}
        </Button>
        <Button
          type="button"
          variant="secondary"
          size="sm"
          disabled={pushModel.isPending}
          onClick={() => setOpen(false)}>
          {t('common.cancel')}
        </Button>
      </div>
      {pushModel.isSuccess && (
        <InlineNotice variant="info">
          {pushModel.data.alreadyPresent
            ? t('backends.push_already_present')
            : t('backends.push_success', {
                name: pushModel.data.name,
                bytes: formatBytes(pushModel.data.bytesWritten) ?? '0 B',
              })}
        </InlineNotice>
      )}
      {pushModel.isError && (
        <InlineNotice variant="error">
          {t('backends.push_error')}: {pushModel.error.message}
        </InlineNotice>
      )}
    </div>
  );
}
