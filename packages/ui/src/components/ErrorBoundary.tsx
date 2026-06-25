import { Component, ErrorInfo, ReactNode } from "react";
import { Button } from "./Button";
import { H1, P } from "./Typography";

interface Props {
  children: ReactNode;
  fallback?: ReactNode | ((error: Error, reset: () => void) => ReactNode);
}

interface State {
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("ErrorBoundary caught:", error, info);
  }

  reset = () => this.setState({ error: null });

  render() {
    if (this.state.error) {
      const { fallback } = this.props;
      if (typeof fallback === "function") {
        return fallback(this.state.error, this.reset);
      }
      return (
        fallback ?? (
          <div className="flex min-h-screen items-center justify-center">
            <div className="text-center space-y-4">
              <H1>Something went wrong</H1>
              <P variant="muted">
                {this.state.error.message}
              </P>
              <Button variant="primary" onClick={this.reset}>
                Try again
              </Button>
            </div>
          </div>
        )
      );
    }
    return this.props.children;
  }
}
