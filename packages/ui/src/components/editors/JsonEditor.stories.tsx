import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { JsonEditor } from "./JsonEditor";

const meta: Meta<typeof JsonEditor> = {
  title: "Editors/JsonEditor",
  component: JsonEditor,
  decorators: [
    (Story) => (
      <div style={{ width: "960px", height: "640px" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof JsonEditor>;

export const ValidJson: Story = {
  render: () => {
    const [value, setValue] = useState<object>({
      name: "Acme Project",
      version: "1.0.0",
      tags: ["alpha", "beta"],
      settings: {
        enabled: true,
        retries: 3,
      },
    });
    return (
      <JsonEditor
        value={value}
        onSave={(next) => setValue(next)}
        onCancel={() => {}}
      />
    );
  },
};

export const Empty: Story = {
  render: () => {
    const [value, setValue] = useState<object>({});
    return (
      <JsonEditor
        value={value}
        onSave={(next) => setValue(next)}
        onCancel={() => {}}
        title="Empty Document"
        description="Start with an empty object and add fields as needed."
      />
    );
  },
};

export const InvalidJsonValidation: Story = {
  render: () => {
    const [value, setValue] = useState<object>({ requiredField: "" });
    return (
      <JsonEditor
        value={value}
        onSave={(next) => setValue(next)}
        onCancel={() => {}}
        title="With Custom Validation"
        description="The save button rejects documents missing requiredField."
        validate={(json) => {
          const obj = json as Record<string, unknown>;
          if (!obj.requiredField || obj.requiredField === "") {
            return { isValid: false, error: "requiredField must be non-empty" };
          }
          return { isValid: true };
        }}
      />
    );
  },
};

export const LargeDocument: Story = {
  render: () => {
    const items = Array.from({ length: 50 }).map((_, i) => ({
      id: `item-${i}`,
      label: `Item number ${i}`,
      score: Math.round(Math.random() * 1000) / 10,
      enabled: i % 2 === 0,
      metadata: {
        createdAt: new Date(Date.UTC(2026, 0, i + 1)).toISOString(),
        owner: `user-${(i % 5) + 1}`,
      },
    }));
    const [value, setValue] = useState<object>({
      total: items.length,
      items,
    });
    return (
      <JsonEditor
        value={value}
        onSave={(next) => setValue(next)}
        onCancel={() => {}}
        title="Large Document"
      />
    );
  },
};
