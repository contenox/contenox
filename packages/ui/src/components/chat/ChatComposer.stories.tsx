import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { ChatComposer } from "./ChatComposer";

const meta: Meta<typeof ChatComposer> = {
  title: "Chat/ChatComposer",
  component: ChatComposer,
  args: {
    placeholder: "Type a message...",
    submitLabel: "Send",
    pendingLabel: "Sending",
  },
};

export default meta;
type Story = StoryObj<typeof ChatComposer>;

export const Empty: Story = {
  render: (args) => {
    const [value, setValue] = useState("");
    return (
      <ChatComposer
        {...args}
        value={value}
        onChange={setValue}
        onSubmit={(e) => {
          e.preventDefault();
          setValue("");
        }}
      />
    );
  },
};

export const WithText: Story = {
  render: (args) => {
    const [value, setValue] = useState(
      "Write a short summary of recent changes to the chains module.",
    );
    return (
      <ChatComposer
        {...args}
        value={value}
        onChange={setValue}
        onSubmit={(e) => e.preventDefault()}
      />
    );
  },
};

export const NearSoftLimit: Story = {
  render: (args) => {
    const softMax = 200;
    const [value, setValue] = useState("x".repeat(180));
    return (
      <ChatComposer
        {...args}
        value={value}
        onChange={setValue}
        softMax={softMax}
        onSubmit={(e) => e.preventDefault()}
      />
    );
  },
};

export const OverSoftLimit: Story = {
  render: (args) => {
    const softMax = 200;
    const [value, setValue] = useState("x".repeat(250));
    return (
      <ChatComposer
        {...args}
        value={value}
        onChange={setValue}
        softMax={softMax}
        softLimitExceededNote="Message exceeds the soft context window. The model may truncate input."
        onSubmit={(e) => e.preventDefault()}
      />
    );
  },
};

export const Pending: Story = {
  render: (args) => {
    const [value, setValue] = useState("Generating answer...");
    return (
      <ChatComposer
        {...args}
        value={value}
        onChange={setValue}
        isPending
        onSubmit={(e) => e.preventDefault()}
      />
    );
  },
};

export const Disabled: Story = {
  render: (args) => {
    const [value, setValue] = useState("");
    return (
      <ChatComposer
        {...args}
        value={value}
        onChange={setValue}
        disabled
        onSubmit={(e) => e.preventDefault()}
      />
    );
  },
};

export const Compact: Story = {
  render: (args) => {
    const [value, setValue] = useState("");
    return (
      <ChatComposer
        {...args}
        variant="compact"
        value={value}
        onChange={setValue}
        onSubmit={(e) => e.preventDefault()}
      />
    );
  },
};

export const Workbench: Story = {
  render: (args) => {
    const [value, setValue] = useState("");
    return (
      <ChatComposer
        {...args}
        variant="workbench"
        title="Workbench Composer"
        value={value}
        onChange={setValue}
        onSubmit={(e) => e.preventDefault()}
      />
    );
  },
};
