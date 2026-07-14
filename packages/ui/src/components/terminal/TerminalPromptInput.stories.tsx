import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { TerminalPromptInput } from "./TerminalPromptInput";

const meta: Meta<typeof TerminalPromptInput> = {
  title: "Terminal/TerminalPromptInput",
  component: TerminalPromptInput,
};
export default meta;
type Story = StoryObj<typeof TerminalPromptInput>;

export const Default: Story = {
  render: () => {
    const [value, setValue] = useState("fix the login bug in auth.go");
    return (
      <div className="bg-surface-50 dark:bg-dark-surface-100 p-3">
        <TerminalPromptInput
          value={value}
          onChange={setValue}
          hint="type a task · / commands · enter to run"
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              setValue("");
            }
          }}
        />
      </div>
    );
  },
};
