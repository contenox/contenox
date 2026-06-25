import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "./Button";
import { ButtonGroup } from "./ButtonGroup";

const meta: Meta<typeof ButtonGroup> = {
  title: "Primitives/ButtonGroup",
  component: ButtonGroup,
};

export default meta;
type Story = StoryObj<typeof ButtonGroup>;

export const Default: Story = {
  render: () => (
    <ButtonGroup>
      <Button variant="primary">Save</Button>
      <Button variant="outline">Cancel</Button>
    </ButtonGroup>
  ),
};

export const Trio: Story = {
  render: () => (
    <ButtonGroup>
      <Button variant="primary">Apply</Button>
      <Button variant="secondary">Reset</Button>
      <Button variant="ghost">Discard</Button>
    </ButtonGroup>
  ),
};

export const DangerPair: Story = {
  render: () => (
    <ButtonGroup>
      <Button variant="danger">Delete</Button>
      <Button variant="outline">Cancel</Button>
    </ButtonGroup>
  ),
};
