import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { DragDropContextProvider } from "./DragDropContext";
import { Draggable } from "./Draggable";
import { Droppable } from "./Droppable";

const meta: Meta<typeof Droppable> = {
  title: "Interactions/DragDrop/Droppable",
  component: Droppable,
};

export default meta;
type Story = StoryObj<typeof Droppable>;

interface Item {
  id: string;
  label: string;
}

type Zones = Record<string, Item[]>;

function Zone({
  id,
  items,
  disabled,
}: {
  id: string;
  items: Item[];
  disabled?: boolean;
}) {
  return (
    <Droppable
      droppableId={id}
      isDropDisabled={disabled}
      className="min-w-[220px] min-h-[180px] p-4 rounded-lg border border-dashed border-surface-300 dark:border-dark-surface-300 bg-surface-100 dark:bg-dark-surface-100"
    >
      <div className="text-sm font-semibold mb-3 text-text-primary dark:text-dark-text-primary">
        {id}
        {disabled ? " (disabled)" : ""}
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
            Drop here
          </div>
        )}
      </div>
    </Droppable>
  );
}

function Demo({
  initial,
  disabledZones = [],
}: {
  initial: Zones;
  disabledZones?: string[];
}) {
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
          <Zone
            key={id}
            id={id}
            items={items}
            disabled={disabledZones.includes(id)}
          />
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
          { id: "s1", label: "Card one" },
          { id: "s2", label: "Card two" },
        ],
        Target: [{ id: "s3", label: "Card three" }],
      }}
    />
  ),
};

export const EmptyZone: Story = {
  render: () => (
    <Demo
      initial={{
        Items: [
          { id: "e1", label: "Movable A" },
          { id: "e2", label: "Movable B" },
        ],
        Empty: [],
      }}
    />
  ),
};

export const MultiZone: Story = {
  render: () => (
    <Demo
      disabledZones={["Locked"]}
      initial={{
        Inbox: [
          { id: "z1", label: "Task A" },
          { id: "z2", label: "Task B" },
        ],
        Working: [{ id: "z3", label: "Task C" }],
        Locked: [{ id: "z4", label: "Frozen item" }],
        Done: [],
      }}
    />
  ),
};
