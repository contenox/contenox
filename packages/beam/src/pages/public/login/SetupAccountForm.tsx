import { Button, Form, FormField, Input, PasswordInput } from '@contenox/ui';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { api } from '../../../lib/api';
import { useLogin } from '../../../hooks/useLogin';

export function SetupAccountForm({ onInitialized }: { onInitialized: () => void }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [localError, setLocalError] = useState<string | null>(null);

  const { mutate: doLogin, isPending: loginPending } = useLogin({
    onSuccess: () => onInitialized(),
  });

  const init = useMutation({
    mutationFn: api.initAccount,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['authSetupStatus'] });
      doLogin({ email: username, password });
    },
  });

  const valid =
    username.trim() !== '' && password.length >= 4 && password === confirm;

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!valid) {
      setLocalError(t('setup_account.invalid', 'Pick a username, password (4+ chars), and confirm.'));
      return;
    }
    setLocalError(null);
    init.mutate({ username: username.trim(), password });
  };

  const busy = init.isPending || loginPending;

  return (
    <Form
      title={t('setup_account.title', 'Create your local account')}
      onSubmit={handleSubmit}
      error={
        localError ||
        (init.error ? (init.error as Error).message : undefined)
      }
      actions={
        <Button type="submit" variant="primary" disabled={busy || !valid}>
          {busy
            ? t('setup_account.submitting', 'Creating…')
            : t('setup_account.submit', 'Create account')}
        </Button>
      }>
      <FormField label={t('setup_account.username', 'Username')} required>
        <Input value={username} onChange={(e) => setUsername(e.target.value)} disabled={busy} />
      </FormField>
      <FormField label={t('setup_account.password', 'Password')} required>
        <PasswordInput
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          disabled={busy}
        />
      </FormField>
      <FormField label={t('setup_account.confirm_password', 'Confirm password')} required>
        <PasswordInput
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
          disabled={busy}
        />
      </FormField>
    </Form>
  );
}
