import { Button, Form, FormField, PasswordInput } from '@contenox/ui';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useLogin } from '../../../hooks/useLogin';

/**
 * Single "Access token" field. The token is a secret, so it uses a
 * PasswordInput (masked) rather than a plain text input; it is POSTed to
 * /ui/login and never stored client-side — the server keeps the session in an
 * HttpOnly cookie the browser cannot read.
 */
export function LoginForm() {
  const { t } = useTranslation();
  const [token, setToken] = useState('');
  const [localError, setLocalError] = useState<string | null>(null);

  const { mutate: loginMutate, isPending, error: loginError } = useLogin();
  const isFormValid = token.trim() !== '';

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!isFormValid) {
      setLocalError(t('login.token_required', 'An access token is required.'));
      return;
    }
    setLocalError(null);
    loginMutate(token.trim());
  };

  return (
    <Form
      title={t('login.title')}
      onSubmit={handleSubmit}
      error={
        localError ||
        (loginError
          ? t('login.error', 'Login error: {{error}}', { error: loginError.message })
          : undefined)
      }
      onError={errorMsg => console.error('Form error:', errorMsg)}
      actions={
        <Button type="submit" variant="primary" disabled={isPending || !isFormValid}>
          {isPending ? t('login.loading') : t('login.submit')}
        </Button>
      }>
      {/* Intro line under the title. FormField's `description` prop is an inline,
          right-aligned hint meant for a few words; a full sentence belongs on its
          own line, so it renders as a muted paragraph here instead. */}
      <p className="text-text-muted dark:text-dark-text-muted text-sm">
        {t('login.description', 'Enter the access token to reach this runtime.')}
      </p>
      <FormField label={t('login.access_token', 'Access token')} required>
        <PasswordInput
          value={token}
          onChange={e => setToken(e.target.value)}
          disabled={isPending}
          autoComplete="current-password"
        />
      </FormField>
    </Form>
  );
}
