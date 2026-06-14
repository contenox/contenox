#!/usr/bin/env node

const { spawnSync } = require("node:child_process");
const fs = require("node:fs");
const path = require("node:path");
const { targetFromEnv } = require("./vscode-targets");

const extensionRoot = path.resolve(__dirname, "..");
const packagePath = path.join(extensionRoot, "package.json");
const target = targetFromEnv();

run(process.execPath, [path.join("scripts", "sync-package-metadata.js")]);
run(npx(), ["tsc", "-p", "."]);
run(process.execPath, [path.join("scripts", "build-binary.js")], {
  CONTENOX_VSCODE_TARGET: target.name,
});

const pkg = JSON.parse(fs.readFileSync(packagePath, "utf8"));
const out =
  process.env.CONTENOX_VSCODE_OUT ||
  path.join(extensionRoot, "artifacts", `${pkg.name}-${target.name}-${pkg.version}.vsix`);
fs.mkdirSync(path.dirname(out), { recursive: true });
fs.rmSync(out, { force: true });

const vsceArgs = [
  "vsce",
  "package",
  "--target",
  target.name,
  "--no-dependencies",
  "--no-yarn",
  "--out",
  out,
];
if (process.env.CONTENOX_VSCODE_SKIP_VSCE_SECRET_SCAN === "1") {
  vsceArgs.splice(vsceArgs.indexOf("--out"), 0, "--allow-package-all-secrets", "--allow-package-env-file");
}

run(npx(), vsceArgs);
console.log(`Built ${target.name} VS Code extension: ${path.relative(extensionRoot, out)}`);

function run(command, args, extraEnv = {}) {
  const result = spawnSync(command, args, {
    cwd: extensionRoot,
    env: {
      ...process.env,
      ...extraEnv,
    },
    stdio: "inherit",
  });
  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }
}

function npx() {
  return process.platform === "win32" ? "npx.cmd" : "npx";
}
