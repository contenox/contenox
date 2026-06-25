import type { Meta, StoryObj } from "@storybook/react-vite";
import { Blockquote, H1, H2, H3, P, Small, Span } from "./Typography";

const meta: Meta = {
  title: "Primitives/Typography",
};

export default meta;
type Story = StoryObj;

export const Default: Story = {
  render: () => (
    <div style={{ display: "flex", flexDirection: "column", gap: "1rem" }}>
      <H1>Heading 1</H1>
      <H2>Heading 2</H2>
      <H3>Heading 3</H3>
      <P>Body paragraph text used for default content.</P>
      <Small>Small helper text</Small>
      <Span>Inline span</Span>
      <Blockquote>A pull quote rendered as a blockquote.</Blockquote>
    </div>
  ),
};

export const H1Variants: Story = {
  render: () => (
    <div style={{ display: "flex", flexDirection: "column", gap: "1rem" }}>
      <H1 variant="hero">Hero heading</H1>
      <H1 variant="page">Page heading</H1>
      <H1 variant="sectionTitle">Section title</H1>
      <H1>Default H1</H1>
    </div>
  ),
};

export const H2Variants: Story = {
  render: () => (
    <div style={{ display: "flex", flexDirection: "column", gap: "1rem" }}>
      <H2 variant="sectionTitle">Section title H2</H2>
      <H2 variant="footerTitle">Footer title</H2>
      <H2>Default H2</H2>
    </div>
  ),
};

export const ParagraphVariants: Story = {
  render: () => (
    <div style={{ display: "flex", flexDirection: "column", gap: "1rem" }}>
      <P variant="lead">Lead paragraph: the opening of a section.</P>
      <P variant="body">Body paragraph text.</P>
      <P variant="cardSubtitle">Card subtitle text.</P>
      <P variant="footerText">Footer paragraph text.</P>
      <P variant="caption">Caption text</P>
      <P variant="status">Status text</P>
    </div>
  ),
};

export const SpanVariants: Story = {
  render: () => (
    <div style={{ display: "flex", flexDirection: "column", gap: "0.5rem" }}>
      <Span>Default span</Span>
      <Span variant="status">Status span</Span>
      <Span variant="muted">Muted span</Span>
    </div>
  ),
};
