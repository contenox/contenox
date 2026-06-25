import type { Meta, StoryObj } from "@storybook/react-vite";
import { Wizard } from "./Wizard";
import { WizardStep, type WizardStepStatus } from "./WizardStep";
import { Button } from "../Button";

const meta: Meta<typeof Wizard> = {
  title: "Wizard/Wizard",
  component: Wizard,
  decorators: [
    (Story) => (
      <div style={{ width: "640px" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof Wizard>;

type StepDef = {
  step: number;
  title: string;
  description: string;
};

const steps: StepDef[] = [
  {
    step: 1,
    title: "Connect a backend",
    description: "Point the runtime at a local or remote model provider.",
  },
  {
    step: 2,
    title: "Select a model",
    description: "Pick a chat model to use for your first session.",
  },
  {
    step: 3,
    title: "Start chatting",
    description: "Open the chat surface and send your first message.",
  },
];

function statusFor(step: number, current: number): WizardStepStatus {
  if (step < current) return "complete";
  if (step === current) return "current";
  return "upcoming";
}

function renderSteps(current: number) {
  return steps.map((s, idx) => (
    <WizardStep
      key={s.step}
      step={s.step}
      status={statusFor(s.step, current)}
      active={s.step === current}
      title={s.title}
      description={s.description}
      isLast={idx === steps.length - 1}
    >
      {s.step === current ? (
        <Button variant="primary" size="sm">
          Continue
        </Button>
      ) : null}
    </WizardStep>
  ));
}

export const Default: Story = {
  render: () => (
    <Wizard
      title="Getting started"
      description="Three quick steps to your first chat."
    >
      {renderSteps(1)}
    </Wizard>
  ),
};

export const Middle: Story = {
  render: () => (
    <Wizard
      title="Getting started"
      description="Three quick steps to your first chat."
    >
      {renderSteps(2)}
    </Wizard>
  ),
};

export const LastStep: Story = {
  render: () => (
    <Wizard
      title="Getting started"
      description="Three quick steps to your first chat."
    >
      {renderSteps(3)}
    </Wizard>
  ),
};

export const Completed: Story = {
  render: () => (
    <Wizard
      title="All set"
      description="You're ready to go."
      footer={
        <div className="flex justify-end">
          <Button variant="primary">Open chat</Button>
        </div>
      }
    >
      {steps.map((s, idx) => (
        <WizardStep
          key={s.step}
          step={s.step}
          status="complete"
          title={s.title}
          description={s.description}
          isLast={idx === steps.length - 1}
        />
      ))}
    </Wizard>
  ),
};
