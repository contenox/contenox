import type { Meta, StoryObj } from "@storybook/react-vite";
import { Form } from "./Form";
import { FormField } from "./FormField";
import { Input } from "./Input";
import { Textarea } from "./TextArea";
import { Select } from "./Select";
import { Checkbox } from "./Checkbox";
import { Button } from "./Button";

const meta: Meta<typeof Form> = {
  title: "Forms/Form",
  component: Form,
  argTypes: {
    variant: {
      control: "select",
      options: [
        "default",
        "raised",
        "flat",
        "bordered",
        "error",
        "gradient",
        "surface",
        "ghost",
        "empty",
        "body",
      ],
    },
  },
  args: {
    title: "Create account",
    variant: "default",
    onSubmit: () => {},
  },
  decorators: [
    (Story) => (
      <div style={{ width: "480px" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof Form>;

export const Default: Story = {
  args: {
    children: (
      <>
        <FormField label="Email" required>
          <Input type="email" placeholder="you@example.com" />
        </FormField>
        <FormField label="Password" required>
          <Input type="password" placeholder="••••••••" />
        </FormField>
      </>
    ),
    actions: (
      <>
        <Button variant="primary" type="submit">
          Sign up
        </Button>
        <Button variant="ghost" type="button">
          Cancel
        </Button>
      </>
    ),
  },
};

export const WithError: Story = {
  args: {
    title: "Login",
    error: "Invalid email or password",
    children: (
      <>
        <FormField label="Email" required>
          <Input type="email" defaultValue="invalid@example" error />
        </FormField>
        <FormField label="Password" required>
          <Input type="password" defaultValue="wrong" error />
        </FormField>
      </>
    ),
    actions: (
      <Button variant="primary" type="submit">
        Log in
      </Button>
    ),
  },
};

export const Composite: Story = {
  args: {
    title: "Project settings",
    children: (
      <>
        <FormField label="Project name" required>
          <Input placeholder="My project" />
        </FormField>
        <FormField label="Description" description="Optional">
          <Textarea rows={3} placeholder="What is this project about?" />
        </FormField>
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
        <Checkbox label="Enable notifications" defaultChecked />
      </>
    ),
    actions: (
      <>
        <Button variant="primary" type="submit">
          Save
        </Button>
        <Button variant="ghost" type="button">
          Discard
        </Button>
      </>
    ),
  },
};

export const NoTitle: Story = {
  args: {
    title: undefined,
    variant: "flat",
    children: (
      <FormField label="Search query">
        <Input placeholder="Type to search..." />
      </FormField>
    ),
    actions: (
      <Button variant="primary" type="submit">
        Search
      </Button>
    ),
  },
};
