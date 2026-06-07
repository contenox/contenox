import type { Meta, StoryObj } from "@storybook/react-vite";
import { TabbedForm } from "./TabbedForm";
import { FormField } from "../FormField";
import { Input } from "../Input";
import { Select } from "../Select";
import { Textarea } from "../TextArea";

const meta: Meta<typeof TabbedForm> = {
  title: "Forms/TabbedForm",
  component: TabbedForm,
  decorators: [
    (Story) => (
      <div style={{ width: "720px", height: "560px" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof TabbedForm>;

export const TwoTabs: Story = {
  args: {
    title: "Project Settings",
    description: "Configure project metadata and access controls.",
    onSave: () => {},
    onCancel: () => {},
    tabs: [
      {
        id: "general",
        label: "General",
        content: (
          <div className="space-y-4 p-4">
            <FormField label="Project name" required>
              <Input placeholder="My project" />
            </FormField>
            <FormField label="Description">
              <Textarea rows={3} placeholder="What is this project about?" />
            </FormField>
          </div>
        ),
      },
      {
        id: "access",
        label: "Access",
        content: (
          <div className="space-y-4 p-4">
            <FormField label="Visibility" required>
              <Select
                placeholder="Choose visibility"
                options={[
                  { value: "private", label: "Private" },
                  { value: "team", label: "Team" },
                  { value: "public", label: "Public" },
                ]}
              />
            </FormField>
          </div>
        ),
      },
    ],
  },
};

export const ThreeTabs: Story = {
  args: {
    title: "Account",
    description: "Manage profile, security, and notifications.",
    onSave: () => {},
    onCancel: () => {},
    onDelete: () => {},
    tabs: [
      {
        id: "profile",
        label: "Profile",
        content: (
          <div className="space-y-4 p-4">
            <FormField label="Full name" required>
              <Input placeholder="Jane Doe" />
            </FormField>
            <FormField label="Email" required>
              <Input type="email" placeholder="you@example.com" />
            </FormField>
          </div>
        ),
      },
      {
        id: "security",
        label: "Security",
        content: (
          <div className="space-y-4 p-4">
            <FormField label="Current password" required>
              <Input type="password" />
            </FormField>
            <FormField label="New password" required>
              <Input type="password" />
            </FormField>
          </div>
        ),
      },
      {
        id: "notifications",
        label: "Notifications",
        content: (
          <div className="space-y-4 p-4">
            <FormField label="Frequency">
              <Select
                placeholder="Choose frequency"
                options={[
                  { value: "realtime", label: "Real-time" },
                  { value: "daily", label: "Daily digest" },
                  { value: "weekly", label: "Weekly digest" },
                ]}
              />
            </FormField>
          </div>
        ),
      },
    ],
  },
};

export const WithDisabledTab: Story = {
  args: {
    title: "Workspace",
    onSave: () => {},
    onCancel: () => {},
    tabs: [
      {
        id: "general",
        label: "General",
        content: (
          <div className="space-y-4 p-4">
            <FormField label="Workspace name">
              <Input placeholder="Acme" />
            </FormField>
          </div>
        ),
      },
      {
        id: "billing",
        label: "Billing",
        disabled: true,
        content: (
          <div className="p-4">
            <p>Billing is currently unavailable.</p>
          </div>
        ),
      },
      {
        id: "danger",
        label: "Danger zone",
        content: (
          <div className="space-y-4 p-4">
            <FormField label="Type DELETE to confirm">
              <Input placeholder="DELETE" />
            </FormField>
          </div>
        ),
      },
    ],
  },
};
