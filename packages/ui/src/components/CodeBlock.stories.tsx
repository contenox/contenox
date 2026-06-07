import type { Meta, StoryObj } from "@storybook/react-vite";
import { CodeBlock } from "./CodeBlock";

const meta: Meta<typeof CodeBlock> = {
  title: "Data/CodeBlock",
  component: CodeBlock,
};

export default meta;
type Story = StoryObj<typeof CodeBlock>;

const jsonSnippet = `{
  "id": "chain-7f3",
  "tasks": [
    { "id": "fetch", "type": "http" },
    { "id": "summarize", "type": "llm", "model": "claude-opus-4-7" }
  ],
  "edges": [["fetch", "summarize"]]
}`;

const longSnippet = `package main

import (
  "context"
  "fmt"
  "github.com/contenox/runtime/libtracker"
)

func main() {
  ctx := context.Background()
  tr := libtracker.NewActivityTracker("demo")
  defer tr.Close()

  for i := 0; i < 50; i++ {
    tr.Track(ctx, "iteration", map[string]any{
      "index": i,
      "phase": "warmup",
    })
    fmt.Printf("step %d done\\n", i)
  }
}`;

export const Default: Story = {
  render: () => <CodeBlock>{jsonSnippet}</CodeBlock>,
};

export const Empty: Story = {
  render: () => <CodeBlock>{""}</CodeBlock>,
};

export const Rich: Story = {
  render: () => <CodeBlock style={{ maxHeight: 320 }}>{longSnippet}</CodeBlock>,
};
