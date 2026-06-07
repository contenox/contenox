import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { Dropdown } from "./Dropdown";
import { Button } from "./Button";

const meta: Meta<typeof Dropdown> = {
  title: "Overlays/Dropdown",
  component: Dropdown,
  decorators: [
    (Story) => (
      <div style={{ padding: "4rem", minHeight: "240px" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof Dropdown>;

const sampleOptions = [
  { value: "one", label: "Option One" },
  { value: "two", label: "Option Two" },
  { value: "three", label: "Option Three" },
];

export const Default: Story = {
  render: () => {
    const [value, setValue] = useState("one");
    return (
      <div style={{ width: "240px" }}>
        <Dropdown options={sampleOptions} value={value} onChange={setValue} />
      </div>
    );
  },
};

export const Open: Story = {
  render: () => {
    const [value, setValue] = useState("two");
    return (
      <div style={{ width: "240px" }}>
        <Dropdown
          isOpen={true}
          onToggle={() => {}}
          options={sampleOptions}
          value={value}
          onChange={setValue}
        />
      </div>
    );
  },
};

export const WithCustomTrigger: Story = {
  render: () => {
    const [open, setOpen] = useState(true);
    return (
      <div style={{ width: "240px" }}>
        <Dropdown
          isOpen={open}
          onToggle={setOpen}
          trigger={<Button variant="primary">Open Menu</Button>}
        >
          <div style={{ padding: "0.5rem" }}>
            <Button variant="ghost" className="w-full text-left">
              Profile
            </Button>
            <Button variant="ghost" className="w-full text-left">
              Settings
            </Button>
            <Button variant="ghost" className="w-full text-left">
              Logout
            </Button>
          </div>
        </Dropdown>
      </div>
    );
  },
};

export const Closed: Story = {
  render: () => {
    const [value, setValue] = useState("one");
    return (
      <div style={{ width: "240px" }}>
        <Dropdown
          isOpen={false}
          onToggle={() => {}}
          options={sampleOptions}
          value={value}
          onChange={setValue}
        />
      </div>
    );
  },
};
