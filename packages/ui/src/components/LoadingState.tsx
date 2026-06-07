import { Button } from "./Button";
import { Panel } from "./Panel";
import { P } from "./Typography";
import { Spinner } from "./Spinner";

import { cn } from "../utils";

export function LoadingState({
  message = "Loading...",
  className,
}: {
  message?: string;
  className?: string;
}) {
  return (
    <div className={cn("flex items-center justify-center py-12", className)}>
      <div className="text-center space-y-4">
        <Spinner size="lg" className="mx-auto" />
        <P variant="muted">{message}</P>
      </div>
    </div>
  );
}

export function ErrorState({
  error,
  onRetry,
  title = "Error",
  description = "An error occurred while loading data.",
}: {
  error?: Error | string;
  onRetry?: () => void;
  title?: string;
  description?: string;
}) {
  return (
    <Panel variant="error" className="p-6">
      <div className="text-center space-y-4">
        <P className="font-medium">{title}</P>
        <P variant="muted">
          {typeof error === "string" ? error : error?.message || description}
        </P>
        {onRetry && (
          <Button variant="outline" onClick={onRetry}>
            Try Again
          </Button>
        )}
      </div>
    </Panel>
  );
}
