import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { DragDropContextProvider } from "./DragDropContext";
import { Draggable } from "./Draggable";
import { Droppable } from "./Droppable";

const meta: Meta<typeof DragDropContextProvider> = {
  title: "Interactions/DragDrop/DragDropContext",
  component: DragDropContextProvider,
};

export default meta;
type Story = StoryObj<typeof DragDropContextProvider>;

interface Item {
  id: string;
  label: string;
}

type Zones = Record<string, Item[]>;

function ZoneList({ id, items, title }: { id: string; items: Item[]; title: string }) {
  return (
    <Droppable
      droppableId={id}
      className="min-w-[220px] min-h-[200px] p-4 rounded-lg border border-dashed border-surface-300 dark:border-dark-surface-300 bg-surface-100 dark:bg-dark-surface-100"
    >
      <div className="text-sm font-semibold mb-3 text-text-primary dark:text-dark-text-primary">
        {title}
      </div>
      <div className="flex flex-col gap-2">
        {items.map((item, index) => (
          <Draggable
            key={item.id}
            draggableId={item.id}
            index={index}
            className="px-3 py-2 rounded-md bg-surface-0 dark:bg-dark-surface-0 border border-surface-200 dark:border-dark-surface-200 text-sm text-text-primary dark:text-dark-text-primary"
          >
            {item.label}
          </Draggable>
        ))}
        {items.length === 0 && (
          <div className="text-xs text-text-secondary dark:text-dark-text-secondary italic">
            Drop items here
          </div>
        )}
      </div>
    </Droppable>
  );
}

function ComposedExample({ initial }: { initial: Zones }) {
  const [zones, setZones] = useState<Zones>(initial);

  const handleDragEnd = (result: {
    draggableId: string;
    sourceDroppableId: string;
    destinationDroppableId: string;
  }) => {
    if (result.sourceDroppableId === result.destinationDroppableId) return;

    setZones((prev) => {
      const next: Zones = {};
      for (const key of Object.keys(prev)) {
        next[key] = [...prev[key]];
      }
      const sourceItems = next[result.sourceDroppableId];
      const destItems = next[result.destinationDroppableId];
      if (!sourceItems || !destItems) return prev;

      const idx = sourceItems.findIndex((i) => i.id === result.draggableId);
      if (idx === -1) return prev;
      const [moved] = sourceItems.splice(idx, 1);
      destItems.push(moved);
      return next;
    });
  };

  return (
    <DragDropContextProvider onDragEnd={handleDragEnd}>
      <div className="flex gap-4 p-4">
        {Object.entries(zones).map(([id, items]) => (
          <ZoneList key={id} id={id} items={items} title={id} />
        ))}
      </div>
    </DragDropContextProvider>
  );
}

export const Default: Story = {
  render: () => (
    <ComposedExample
      initial={{
        Todo: [
          { id: "t1", label: "Write specification" },
          { id: "t2", label: "Review pull request" },
          { id: "t3", label: "Deploy to staging" },
        ],
        Done: [{ id: "t4", label: "Initial scaffolding" }],
      }}
    />
  ),
};

export const EmptyZone: Story = {
  render: () => (
    <ComposedExample
      initial={{
        Backlog: [
          { id: "e1", label: "Item A" },
          { id: "e2", label: "Item B" },
        ],
        Empty: [],
      }}
    />
  ),
};

export const MultiZone: Story = {
  render: () => (
    <ComposedExample
      initial={{
        Inbox: [
          { id: "m1", label: "Email from product" },
          { id: "m2", label: "Bug report" },
        ],
        InProgress: [{ id: "m3", label: "Design review" }],
        Review: [],
        Shipped: [{ id: "m4", label: "Release notes" }],
      }}
    />
  ),
};
