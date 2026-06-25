import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { DetailsPanel } from "./DetailsPanel";

const meta: Meta<typeof DetailsPanel> = {
  title: "Panels/DetailsPanel",
  component: DetailsPanel,
  decorators: [
    (Story) => (
      <div style={{ height: "640px", display: "flex", justifyContent: "flex-end" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof DetailsPanel>;

export const OpenWithContent: Story = {
  render: () => {
    const [data, setData] = useState<Record<string, unknown>>({
      name: "Production Pipeline",
      description: "Main build and deploy pipeline for production releases.",
      status: "active",
      owner: "platform-team",
    });
    return (
      <DetailsPanel
        title="Pipeline Details"
        data={data}
        fields={[
          { key: "name", label: "Name", type: "text" },
          { key: "description", label: "Description", type: "textarea" },
          {
            key: "status",
            label: "Status",
            type: "select",
            options: [
              { value: "active", label: "Active" },
              { value: "paused", label: "Paused" },
              { value: "archived", label: "Archived" },
            ],
          },
          { key: "owner", label: "Owner", type: "badge" },
        ]}
        onClose={() => {}}
        onSave={(next) => setData(next)}
        onDelete={() => {}}
      />
    );
  },
};

export const EditMode: Story = {
  render: () => {
    const [data, setData] = useState<Record<string, unknown>>({
      name: "Staging Pipeline",
      description: "Pre-production validation.",
      status: "paused",
    });
    return (
      <DetailsPanel
        title="Edit Pipeline"
        data={data}
        fields={[
          { key: "name", label: "Name", type: "text" },
          { key: "description", label: "Description", type: "textarea" },
          {
            key: "status",
            label: "Status",
            type: "select",
            options: [
              { value: "active", label: "Active" },
              { value: "paused", label: "Paused" },
            ],
          },
        ]}
        isEditing
        onClose={() => {}}
        onSave={(next) => setData(next)}
      />
    );
  },
};

export const EmptyData: Story = {
  render: () => (
    <DetailsPanel
      title="No Selection"
      data={{}}
      fields={[
        { key: "name", label: "Name", type: "text" },
        { key: "notes", label: "Notes", type: "textarea" },
      ]}
      onClose={() => {}}
    />
  ),
};
