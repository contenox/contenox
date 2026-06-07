import type { Meta, StoryObj } from "@storybook/react-vite";
import { ErrorState, LoadingState } from "./LoadingState";

const meta: Meta<typeof LoadingState> = {
  title: "Primitives/LoadingState",
  component: LoadingState,
  args: {
    message: "Loading...",
  },
};

export default meta;
type Story = StoryObj<typeof LoadingState>;

export const Default: Story = {};

export const CustomMessage: Story = {
  args: { message: "Fetching data, please wait..." },
};

export const ErrorStateDefault: StoryObj<typeof ErrorState> = {
  render: () => <ErrorState />,
};

export const ErrorStateWithMessage: StoryObj<typeof ErrorState> = {
  render: () => (
    <ErrorState
      title="Request failed"
      error="The server returned a 500 error."
    />
  ),
};

export const ErrorStateWithRetry: StoryObj<typeof ErrorState> = {
  render: () => (
    <ErrorState
      title="Network error"
      error="Could not reach the server."
      onRetry={() => {}}
    />
  ),
};
