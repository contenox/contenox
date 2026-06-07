import type { Preview } from "@storybook/react-vite";
import { withThemeByClassName } from "@storybook/addon-themes";
import React from "react";
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
    withThemeByClassName({
      themes: { light: "", dark: "dark" },
      defaultTheme: "light",
      parentSelector: "body",
    }),
    (Story, context) => {
      const isDark = context.globals.theme === "dark";
      const themeVars = isDark
        ? {
            ["--background" as string]: "var(--color-dark-background-light)",
            ["--foreground" as string]: "var(--color-dark-text-primary)",
          }
        : {
            ["--background" as string]: "var(--color-surface-50)",
            ["--foreground" as string]: "var(--color-text)",
          };
      return React.createElement(
        "div",
        {
          style: {
            ...themeVars,
            minHeight: "100vh",
            padding: "1.5rem",
            background: "var(--background)",
            color: "var(--foreground)",
            fontFamily: "var(--font-body)",
          },
        },
        React.createElement(Story),
      );
    },
  ],
};

export default preview;
