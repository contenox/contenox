import type { Meta, StoryObj } from "@storybook/react-vite";
import { WizardStep } from "./WizardStep";
import { Button } from "../Button";

const meta: Meta<typeof WizardStep> = {
  title: "Wizard/WizardStep",
  component: WizardStep,
  argTypes: {
    status: {
      control: "select",
      options: ["complete", "current", "upcoming", "error"],
    },
    active: { control: "boolean" },
    isLast: { control: "boolean" },
  },
  args: {
    step: 1,
    status: "current",
    title: "Connect a backend",
    description: "Point the runtime at a local or remote model provider.",
    isLast: true,
  },
  decorators: [
    (Story) => (
      <div style={{ width: "560px" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof WizardStep>;

export const Current: Story = {
  args: {
    children: (
      <Button variant="primary" size="sm">
        Continue
      </Button>
    ),
  },
};

export const Complete: Story = {
  args: {
    status: "complete",
    title: "Backend connected",
    description: "Connected to local Ollama at http://localhost:11434.",
  },
};

export const Upcoming: Story = {
  args: {
    step: 3,
    status: "upcoming",
    title: "Start chatting",
    description: "Open the chat surface and send your first message.",
  },
};

export const ErrorState: Story = {
  args: {
    status: "error",
    active: true,
    title: "Backend unreachable",
    description: "Could not reach the configured backend. Check the URL and retry.",
    children: (
      <Button variant="secondary" size="sm">
        Retry
      </Button>
    ),
  },
};

export const WithConnector: Story = {
  args: {
    isLast: false,
    status: "complete",
    title: "Step with connector",
    description: "Renders a connecting line below the indicator.",
  },
};
