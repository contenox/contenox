import type { Preview } from "@storybook/react-vite";
import { withThemeByClassName } from "@storybook/addon-themes";
import React from "react";
import "../src/fonts.css";
import "../src/index.css";

const preview: Preview = {
  parameters: {
    controls: {
      matchers: {
        color: /(background|color)$/i,
        date: /Date$/i,
      },
    },
    backgrounds: { disable: true },
  },
  decorators: [
    // Apply .dark to <html> — same element beam's ThemeProvider targets, so
    // the .dark token flip AND html.dark color-scheme both engage.
    withThemeByClassName({
      themes: { light: "", dark: "dark" },
      defaultTheme: "light",
      parentSelector: "html",
    }),
    (Story) =>
      // --background/--foreground come from index.css (:root / .dark); no
      // manual overrides here or broken dark styling gets masked.
      React.createElement(
        "div",
        {
          style: {
            minHeight: "100vh",
            padding: "1.5rem",
            background: "var(--background)",
            color: "var(--foreground)",
            fontFamily: "var(--font-body)",
          },
        },
        React.createElement(Story),
      ),
  ],
};

export default preview;
