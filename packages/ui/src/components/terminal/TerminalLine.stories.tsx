import type { Meta, StoryObj } from "@storybook/react-vite";
import { TerminalLine, TerminalMeta } from "./TerminalLine";
import { TERMINAL_GLYPH } from "./glyphs";

const meta: Meta<typeof TerminalLine> = {
  title: "Terminal/TerminalLine",
  component: TerminalLine,
};
export default meta;
type Story = StoryObj<typeof TerminalLine>;

export const Scrollback: Story = {
  render: () => (
    <div className="bg-surface-50 dark:bg-dark-surface-100 space-y-0.5 p-3 text-[13px]">
      <TerminalLine glyph={TERMINAL_GLYPH.prompt}>run the migration</TerminalLine>
      <TerminalMeta>default-chain.json · qwen2.5-coder · 1,204 tok</TerminalMeta>
      <TerminalLine glyph={TERMINAL_GLYPH.tool} indent={1}>
        local_shell(psql -f migrate.sql)
      </TerminalLine>
      <TerminalLine glyph={TERMINAL_GLYPH.cont} glyphTone="muted" tone="muted" indent={1}>
        ALTER TABLE ... OK
      </TerminalLine>
      <TerminalLine glyph={TERMINAL_GLYPH.approval} glyphTone="muted" tone="success" indent={1}>
        write_file — approved
      </TerminalLine>
      <TerminalLine tone="error" indent={2}>
        [step "verify" failed: exit 1]
      </TerminalLine>
    </div>
  ),
};
