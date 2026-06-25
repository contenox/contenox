import type { Meta, StoryObj } from "@storybook/react-vite";
import { Accordion } from "./Accordion";
import { P } from "./Typography";

const meta: Meta<typeof Accordion> = {
  title: "Layout/Accordion",
  component: Accordion,
  args: {
    title: "Section title",
    children: <P>Accordion body content.</P>,
  },
};

export default meta;
type Story = StoryObj<typeof Accordion>;

export const Closed: Story = {};

export const WithLongContent: Story = {
  args: {
    title: "Advanced settings",
    children: (
      <>
        <P>First detail paragraph inside the accordion body.</P>
        <P>Second detail paragraph with more information.</P>
      </>
    ),
  },
};

export const Multiple: Story = {
  render: () => (
    <div style={{ display: "flex", flexDirection: "column", gap: "0.5rem", width: 480 }}>
      <Accordion title="General">
        <P>General settings.</P>
      </Accordion>
      <Accordion title="Networking">
        <P>Networking settings.</P>
      </Accordion>
      <Accordion title="Security">
        <P>Security settings.</P>
      </Accordion>
    </div>
  ),
};
