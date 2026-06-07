import { Card, Page, Spinner } from '@contenox/ui';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../../lib/api';
import { LoginForm } from './LoginForm';
import { SetupAccountForm } from './SetupAccountForm';

export default function AuthPage() {
  const queryClient = useQueryClient();
  const { data, isLoading } = useQuery({
    queryKey: ['authSetupStatus'],
    queryFn: api.getAuthSetupStatus,
    staleTime: 0,
  });

  const showSetup = !isLoading && data && !data.initialized;

  return (
    <Page bodyScroll="hidden">
      <div className="flex min-h-screen flex-col justify-start py-16">
        <Card className="w-full max-w-4xl min-w-xs place-self-center" variant="filled">
          {isLoading ? (
            <div className="flex items-center justify-center p-8">
              <Spinner size="md" />
            </div>
          ) : showSetup ? (
            <SetupAccountForm
              onInitialized={() => {
                void queryClient.invalidateQueries({ queryKey: ['authSetupStatus'] });
              }}
            />
          ) : (
            <LoginForm />
          )}
        </Card>
      </div>
    </Page>
  );
}
