import type { Meta, StoryObj } from "@storybook/react-vite";
import { FileTree, type FileTreeNode } from "./FileTree";

const meta: Meta<typeof FileTree> = {
  title: "Data/FileTree",
  component: FileTree,
};

export default meta;
type Story = StoryObj<typeof FileTree>;

const smallTree: FileTreeNode[] = [
  {
    id: "src",
    name: "src",
    isDirectory: true,
    children: [
      { id: "src/index.ts", name: "index.ts", path: "src/index.ts" },
      { id: "src/utils.ts", name: "utils.ts", path: "src/utils.ts" },
      {
        id: "src/components",
        name: "components",
        isDirectory: true,
        children: [
          { id: "src/components/Button.tsx", name: "Button.tsx" },
          { id: "src/components/Table.tsx", name: "Table.tsx" },
        ],
      },
    ],
  },
  { id: "package.json", name: "package.json" },
  { id: "README.md", name: "README.md" },
];

const richTree: FileTreeNode[] = [
  {
    id: "packages",
    name: "packages",
    isDirectory: true,
    children: [
      {
        id: "packages/ui",
        name: "ui",
        isDirectory: true,
        children: [
          {
            id: "packages/ui/src",
            name: "src",
            isDirectory: true,
            children: [
              {
                id: "packages/ui/src/components",
                name: "components",
                isDirectory: true,
                children: [
                  { id: "comp-Button", name: "Button.tsx" },
                  { id: "comp-Card", name: "Card.tsx" },
                  { id: "comp-Table", name: "Table.tsx" },
                  { id: "comp-CodeBlock", name: "CodeBlock.tsx" },
                  { id: "comp-DiffView", name: "DiffView.tsx" },
                  { id: "comp-FileTree", name: "FileTree.tsx" },
                ],
              },
              { id: "packages/ui/src/utils.ts", name: "utils.ts" },
              { id: "packages/ui/src/index.ts", name: "index.ts" },
            ],
          },
          { id: "packages/ui/package.json", name: "package.json" },
          { id: "packages/ui/tsconfig.json", name: "tsconfig.json" },
        ],
      },
      {
        id: "packages/beam",
        name: "beam",
        isDirectory: true,
        children: [
          { id: "packages/beam/src", name: "src", isDirectory: true, children: [] },
          { id: "packages/beam/package.json", name: "package.json" },
        ],
      },
    ],
  },
  { id: "go.mod", name: "go.mod" },
  { id: "go.sum", name: "go.sum" },
  { id: "Makefile", name: "Makefile" },
];

export const Default: Story = {
  render: () => (
    <div style={{ width: 320 }}>
      <FileTree nodes={smallTree} selectedId="src/index.ts" />
    </div>
  ),
};

export const Empty: Story = {
  render: () => (
    <div style={{ width: 320 }}>
      <FileTree nodes={[]} />
    </div>
  ),
};

export const Rich: Story = {
  render: () => (
    <div style={{ width: 360 }}>
      <FileTree nodes={richTree} selectedId="comp-Table" />
    </div>
  ),
};

export const NavigateMode: Story = {
  render: () => (
    <div style={{ width: 360 }}>
      <FileTree nodes={richTree} directoryClickMode="navigate" />
    </div>
  ),
};
