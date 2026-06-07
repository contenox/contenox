import type { Meta, StoryObj } from "@storybook/react-vite";
import { ErrorBoundary } from "./ErrorBoundary";

interface BoomProps {
  shouldThrow?: boolean;
  message?: string;
}

const Boom = ({ shouldThrow = false, message = "Simulated render error" }: BoomProps) => {
  if (shouldThrow) {
    throw new Error(message);
  }
  return <div style={{ padding: "1rem" }}>Child rendered successfully.</div>;
};

const meta: Meta<typeof ErrorBoundary> = {
  title: "Utilities/ErrorBoundary",
  component: ErrorBoundary,
};

export default meta;
type Story = StoryObj<typeof ErrorBoundary>;

export const Healthy: Story = {
  render: () => (
    <ErrorBoundary>
      <Boom shouldThrow={false} />
    </ErrorBoundary>
  ),
};

export const Caught: Story = {
  render: () => (
    <ErrorBoundary>
      <Boom shouldThrow message="Something exploded during render" />
    </ErrorBoundary>
  ),
};

export const CustomFallback: Story = {
  render: () => (
    <ErrorBoundary
      fallback={(error, reset) => (
        <div style={{ padding: "1rem", border: "1px solid #c33", borderRadius: 8 }}>
          <strong>Custom fallback:</strong> {error.message}
          <div style={{ marginTop: "0.5rem" }}>
            <button onClick={reset}>Reset</button>
          </div>
        </div>
      )}
    >
      <Boom shouldThrow message="Boom from custom fallback story" />
    </ErrorBoundary>
  ),
};

export const StaticFallback: Story = {
  render: () => (
    <ErrorBoundary
      fallback={<div style={{ padding: "1rem" }}>Static fallback node shown.</div>}
    >
      <Boom shouldThrow message="Boom from static fallback story" />
    </ErrorBoundary>
  ),
};
