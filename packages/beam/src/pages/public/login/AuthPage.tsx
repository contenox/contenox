import { Card, Page } from '@contenox/ui';
import { LoginForm } from './LoginForm';

/**
 * The remote-access login gate. Rendered by App.tsx's AuthGate (not as a routed
 * page) whenever the server requires a token and this browser has no valid
 * session cookie. A single "Access token" field POSTs /ui/login; on success the
 * server sets an HttpOnly session cookie and the gate re-queries /ui/auth-status
 * and swaps in the app.
 */
export default function AuthPage() {
  return (
    <Page bodyScroll="hidden">
      <div className="flex min-h-screen flex-col justify-start py-16">
        <Card className="w-full max-w-md min-w-xs place-self-center" variant="filled">
          <LoginForm />
        </Card>
      </div>
    </Page>
  );
}
