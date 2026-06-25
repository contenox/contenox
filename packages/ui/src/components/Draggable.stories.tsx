import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { DragDropContextProvider } from "./DragDropContext";
import { Draggable } from "./Draggable";
import { Droppable } from "./Droppable";

const meta: Meta<typeof Draggable> = {
  title: "Interactions/DragDrop/Draggable",
  component: Draggable,
};

export default meta;
type Story = StoryObj<typeof Draggable>;

interface Item {
  id: string;
  label: string;
}

type Zones = Record<string, Item[]>;

function Zone({ id, items }: { id: string; items: Item[] }) {
  return (
    <Droppable
      droppableId={id}
      className="min-w-[220px] min-h-[180px] p-4 rounded-lg border border-dashed border-surface-300 dark:border-dark-surface-300 bg-surface-100 dark:bg-dark-surface-100"
    >
      <div className="text-sm font-semibold mb-3 text-text-primary dark:text-dark-text-primary">
        {id}
      </div>
      <div className="flex flex-col gap-2">
        {items.map((item, index) => (
          <Draggable
            key={item.id}
            draggableId={item.id}
            index={index}
            isDragDisabled={item.id === "locked"}
            className="px-3 py-2 rounded-md bg-surface-0 dark:bg-dark-surface-0 border border-surface-200 dark:border-dark-surface-200 text-sm text-text-primary dark:text-dark-text-primary"
          >
            {item.label}
          </Draggable>
        ))}
      </div>
    </Droppable>
  );
}

function Demo({ initial }: { initial: Zones }) {
  const [zones, setZones] = useState<Zones>(initial);
  const handleDragEnd = (result: {
    draggableId: string;
    sourceDroppableId: string;
    destinationDroppableId: string;
  }) => {
    if (result.sourceDroppableId === result.destinationDroppableId) return;
    setZones((prev) => {
      const next: Zones = {};
      for (const key of Object.keys(prev)) next[key] = [...prev[key]];
      const src = next[result.sourceDroppableId];
      const dest = next[result.destinationDroppableId];
      if (!src || !dest) return prev;
      const idx = src.findIndex((i) => i.id === result.draggableId);
      if (idx === -1) return prev;
      const [moved] = src.splice(idx, 1);
      dest.push(moved);
      return next;
    });
  };

  return (
    <DragDropContextProvider onDragEnd={handleDragEnd}>
      <div className="flex gap-4 p-4">
        {Object.entries(zones).map(([id, items]) => (
          <Zone key={id} id={id} items={items} />
        ))}
      </div>
    </DragDropContextProvider>
  );
}

export const Default: Story = {
  render: () => (
    <Demo
      initial={{
        Source: [
          { id: "d1", label: "Draggable one" },
          { id: "d2", label: "Draggable two" },
          { id: "locked", label: "Locked (drag disabled)" },
        ],
        Target: [],
      }}
    />
  ),
};

export const EmptyZone: Story = {
  render: () => (
    <Demo
      initial={{
        Items: [{ id: "only", label: "Only draggable" }],
        Empty: [],
      }}
    />
  ),
};

export const MultiZone: Story = {
  render: () => (
    <Demo
      initial={{
        A: [
          { id: "a1", label: "Alpha" },
          { id: "a2", label: "Beta" },
        ],
        B: [{ id: "b1", label: "Gamma" }],
        C: [],
      }}
    />
  ),
};
