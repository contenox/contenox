import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { Tabs, type Tab } from "./Tabs";
import { TabPanel, TabPanels } from "./TabPanel";
import { P } from "./Typography";

const meta: Meta<typeof Tabs> = {
  title: "Layout/Tabs",
  component: Tabs,
  decorators: [
    (Story) => (
      <div style={{ width: "640px" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof Tabs>;

const defaultTabs: readonly Tab[] = [
  { id: "overview", label: "Overview" },
  { id: "activity", label: "Activity" },
  { id: "settings", label: "Settings" },
  { id: "members", label: "Members" },
];

export const Default: Story = {
  render: () => {
    const [active, setActive] = useState<string>("overview");
    return (
      <div className="flex flex-col gap-4">
        <Tabs tabs={defaultTabs} activeTab={active} onTabChange={setActive} />
        <TabPanels>
          <TabPanel tabId="overview" activeTab={active}>
            <P>High-level summary of the project state and recent activity.</P>
          </TabPanel>
          <TabPanel tabId="activity" activeTab={active}>
            <P>Stream of recent events, commits, and deployments.</P>
          </TabPanel>
          <TabPanel tabId="settings" activeTab={active}>
            <P>Configuration, integrations, and access controls.</P>
          </TabPanel>
          <TabPanel tabId="members" activeTab={active}>
            <P>People with access to this workspace.</P>
          </TabPanel>
        </TabPanels>
      </div>
    );
  },
};

export const ManyTabs: Story = {
  render: () => {
    const tabs: readonly Tab[] = [
      { id: "one", label: "One" },
      { id: "two", label: "Two" },
      { id: "three", label: "Three" },
      { id: "four", label: "Four" },
      { id: "five", label: "Five" },
      { id: "six", label: "Six" },
      { id: "seven", label: "Seven" },
      { id: "eight", label: "Eight" },
    ];
    const [active, setActive] = useState<string>("one");
    return (
      <div className="flex flex-col gap-4">
        <Tabs tabs={tabs} activeTab={active} onTabChange={setActive} />
        <TabPanels>
          {tabs.map((t) => (
            <TabPanel key={t.id} tabId={t.id} activeTab={active}>
              <P>Content for tab {String(t.label)}.</P>
            </TabPanel>
          ))}
        </TabPanels>
      </div>
    );
  },
};

export const WithIcons: Story = {
  render: () => {
    const tabs: readonly Tab[] = [
      {
        id: "home",
        label: (
          <span className="inline-flex items-center gap-2">
            <span aria-hidden>HOME</span>
            <span>Home</span>
          </span>
        ),
      },
      {
        id: "inbox",
        label: (
          <span className="inline-flex items-center gap-2">
            <span aria-hidden>INBOX</span>
            <span>Inbox</span>
          </span>
        ),
      },
      {
        id: "profile",
        label: (
          <span className="inline-flex items-center gap-2">
            <span aria-hidden>USER</span>
            <span>Profile</span>
          </span>
        ),
      },
    ];
    const [active, setActive] = useState<string>("home");
    return (
      <div className="flex flex-col gap-4">
        <Tabs tabs={tabs} activeTab={active} onTabChange={setActive} />
        <TabPanels>
          <TabPanel tabId="home" activeTab={active}>
            <P>Welcome home.</P>
          </TabPanel>
          <TabPanel tabId="inbox" activeTab={active}>
            <P>You have no unread messages.</P>
          </TabPanel>
          <TabPanel tabId="profile" activeTab={active}>
            <P>Edit your profile and preferences.</P>
          </TabPanel>
        </TabPanels>
      </div>
    );
  },
};

export const WithDisabledTab: Story = {
  render: () => {
    const tabs: readonly Tab[] = [
      { id: "general", label: "General" },
      { id: "billing", label: "Billing", disabled: true },
      { id: "danger", label: "Danger zone" },
    ];
    const [active, setActive] = useState<string>("general");
    return (
      <div className="flex flex-col gap-4">
        <Tabs tabs={tabs} activeTab={active} onTabChange={setActive} />
        <TabPanels>
          <TabPanel tabId="general" activeTab={active}>
            <P>General workspace settings.</P>
          </TabPanel>
          <TabPanel tabId="billing" activeTab={active}>
            <P>Billing is currently unavailable.</P>
          </TabPanel>
          <TabPanel tabId="danger" activeTab={active}>
            <P>Irreversible operations live here.</P>
          </TabPanel>
        </TabPanels>
      </div>
    );
  },
};

export const LazyPanels: Story = {
  render: () => {
    const [active, setActive] = useState<string>("a");
    const tabs: readonly Tab[] = [
      { id: "a", label: "Light" },
      { id: "b", label: "Heavy" },
      { id: "c", label: "Other" },
    ];
    return (
      <div className="flex flex-col gap-4">
        <Tabs tabs={tabs} activeTab={active} onTabChange={setActive} />
        <TabPanels>
          <TabPanel tabId="a" activeTab={active} lazy>
            <P>Light panel — mounted only when active.</P>
          </TabPanel>
          <TabPanel tabId="b" activeTab={active} lazy>
            <P>Heavy panel — mounted only when active.</P>
          </TabPanel>
          <TabPanel tabId="c" activeTab={active} lazy>
            <P>Other panel — mounted only when active.</P>
          </TabPanel>
        </TabPanels>
      </div>
    );
  },
};
