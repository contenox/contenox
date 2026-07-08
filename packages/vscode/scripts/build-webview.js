#!/usr/bin/env node

const fs = require("node:fs");
const path = require("node:path");
const esbuild = require("esbuild");
const postcss = require("postcss");
const tailwindcss = require("@tailwindcss/postcss");

const root = path.resolve(__dirname, "..");
const outDir = path.join(root, "media", "chat");

async function buildScript() {
  await esbuild.build({
    entryPoints: [path.join(root, "webview-src", "chat-entry.tsx")],
    bundle: true,
    outfile: path.join(outDir, "webview.js"),
    format: "iife",
    platform: "browser",
    target: "es2022",
    jsx: "automatic",
    sourcemap: true,
    minify: true,
    logLevel: "info",
    // Force a single React copy. The webview entry (packages/vscode) and BeamChat /
    // @contenox/ui (packages/beam, packages/ui) resolve their own react from different
    // node_modules; bundling two copies gives a null hooks dispatcher and every hook
    // throws "Cannot read properties of null (reading 'useState')", leaving a blank webview.
    alias: {
      react: path.join(root, "node_modules", "react"),
      "react-dom": path.join(root, "node_modules", "react-dom"),
    },
  });
}

async function buildStyles() {
  // @contenox/ui ships a prebuilt Tailwind stylesheet (dist/index.css) that already
  // contains the utility classes ui's own components use (e.g. bg-surface-100), driven
  // by custom --color-* theme tokens defined in ui's source. Our own tailwind pass below
  // only sees webview-src/beam/ui *source*, not ui's compiled @theme output, so it can't
  // regenerate those custom-token utilities itself -- we concatenate ui's prebuilt CSS to
  // cover them, same as beam's own app does by importing '@contenox/ui/styles.css'.
  const uiDistCss = fs.readFileSync(
    path.join(root, "..", "ui", "dist", "index.css"),
    "utf8",
  );

  const cssEntry = path.join(root, "webview-src", "webview.css");
  const source = fs.readFileSync(cssEntry, "utf8");
  const result = await postcss([tailwindcss()]).process(source, {
    from: cssEntry,
    to: path.join(outDir, "webview.css"),
  });
  fs.writeFileSync(path.join(outDir, "webview.css"), `${uiDistCss}\n${result.css}`);
}

async function main() {
  fs.mkdirSync(outDir, { recursive: true });
  await buildScript();
  await buildStyles();
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
