import type { Meta, StoryObj } from "@storybook/react-vite";
import { TabbedPage } from "./TabbedPage";
import { P } from "./Typography";

const meta: Meta<typeof TabbedPage> = {
  title: "Layout/TabbedPage",
  component: TabbedPage,
};

export default meta;
type Story = StoryObj<typeof TabbedPage>;

const sampleTabs = [
  { id: "overview", label: "Overview", content: <P>Overview pane content.</P> },
  { id: "settings", label: "Settings", content: <P>Settings pane content.</P> },
  { id: "history", label: "History", content: <P>History pane content.</P> },
];

export const Default: Story = {
  args: {
    tabs: sampleTabs,
  },
};

export const DefaultActiveTabSettings: Story = {
  args: {
    tabs: sampleTabs,
    defaultActiveTab: "settings",
  },
};

export const TwoTabs: Story = {
  args: {
    tabs: [
      { id: "input", label: "Input", content: <P>Input editor.</P> },
      { id: "output", label: "Output", content: <P>Output preview.</P> },
    ],
  },
};

export const MountActivePanelOnly: Story = {
  args: {
    tabs: sampleTabs,
    mountActivePanelOnly: true,
  },
};
