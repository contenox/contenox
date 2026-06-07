import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { SearchBar } from "./SearchBar";

const meta: Meta<typeof SearchBar> = {
  title: "Forms/SearchBar",
  component: SearchBar,
  args: {
    placeholder: "Search...",
  },
  decorators: [
    (Story) => (
      <div style={{ width: "360px" }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof SearchBar>;

const Controlled = (args: React.ComponentProps<typeof SearchBar>) => {
  const [value, setValue] = useState<string>(String(args.value ?? ""));
  return (
    <SearchBar
      {...args}
      value={value}
      onChange={(e) => setValue(e.target.value)}
      onClear={() => setValue("")}
    />
  );
};

export const Empty: Story = {
  render: (args) => <Controlled {...args} />,
  args: { value: "" },
};

export const WithQuery: Story = {
  render: (args) => <Controlled {...args} />,
  args: { value: "react components" },
};

const items = [
  "Button",
  "Badge",
  "Input",
  "Checkbox",
  "Select",
  "TextArea",
  "Tooltip",
];

const ResultsExample = () => {
  const [query, setQuery] = useState("b");
  const filtered = items.filter((i) =>
    i.toLowerCase().includes(query.toLowerCase()),
  );
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "0.75rem" }}>
      <SearchBar
        placeholder="Search components"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        onClear={() => setQuery("")}
      />
      {filtered.length > 0 ? (
        <ul
          style={{
            listStyle: "none",
            padding: 0,
            margin: 0,
            display: "flex",
            flexDirection: "column",
            gap: "0.25rem",
          }}
        >
          {filtered.map((item) => (
            <li
              key={item}
              style={{
                padding: "0.5rem 0.75rem",
                borderRadius: "0.5rem",
                background: "var(--color-surface-100, #f3f4f6)",
                fontSize: "0.875rem",
              }}
            >
              {item}
            </li>
          ))}
        </ul>
      ) : (
        <p style={{ fontSize: "0.875rem", color: "#6b7280", margin: 0 }}>
          No results
        </p>
      )}
    </div>
  );
};

export const WithResults: Story = {
  render: () => <ResultsExample />,
};

const EmptyResultsExample = () => {
  const [query, setQuery] = useState("nonexistent");
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "0.75rem" }}>
      <SearchBar
        placeholder="Search components"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        onClear={() => setQuery("")}
      />
      <p style={{ fontSize: "0.875rem", color: "#6b7280", margin: 0 }}>
        No results found for "{query}"
      </p>
    </div>
  );
};

export const EmptyResults: Story = {
  render: () => <EmptyResultsExample />,
};
