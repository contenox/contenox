import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { TabPanel, TabPanels } from "./TabPanel";
import { Tabs, type Tab } from "./Tabs";
import { P } from "./Typography";

const meta: Meta<typeof TabPanel> = {
  title: "Layout/Tabs/TabPanel",
  component: TabPanel,
  decorators: [
    (Story) => (
      <div style={{ width: "640px" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof TabPanel>;

const tabs: readonly Tab[] = [
  { id: "details", label: "Details" },
  { id: "logs", label: "Logs" },
  { id: "metrics", label: "Metrics" },
];

export const Default: Story = {
  render: () => {
    const [active, setActive] = useState<string>("details");
    return (
      <div className="flex flex-col gap-4">
        <Tabs tabs={tabs} activeTab={active} onTabChange={setActive} />
        <TabPanels>
          <TabPanel tabId="details" activeTab={active}>
            <P>Details about this resource.</P>
          </TabPanel>
          <TabPanel tabId="logs" activeTab={active}>
            <P>Recent log entries appear here.</P>
          </TabPanel>
          <TabPanel tabId="metrics" activeTab={active}>
            <P>Throughput and latency charts.</P>
          </TabPanel>
        </TabPanels>
      </div>
    );
  },
};

export const Lazy: Story = {
  render: () => {
    const [active, setActive] = useState<string>("details");
    return (
      <div className="flex flex-col gap-4">
        <Tabs tabs={tabs} activeTab={active} onTabChange={setActive} />
        <TabPanels>
          <TabPanel tabId="details" activeTab={active} lazy>
            <P>Details panel — mounted only while active.</P>
          </TabPanel>
          <TabPanel tabId="logs" activeTab={active} lazy>
            <P>Logs panel — unmounts when leaving the tab.</P>
          </TabPanel>
          <TabPanel tabId="metrics" activeTab={active} lazy>
            <P>Metrics panel — avoids running expensive charts off-screen.</P>
          </TabPanel>
        </TabPanels>
      </div>
    );
  },
};

export const Single: Story = {
  render: () => (
    <TabPanel tabId="only" activeTab="only">
      <P>A standalone panel rendered with matching activeTab.</P>
    </TabPanel>
  ),
};
