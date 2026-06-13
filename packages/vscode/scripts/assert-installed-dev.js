#!/usr/bin/env node

const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");

const extensionID = process.env.CONTENOX_VSCODE_EXTENSION_ID || "contenox.runtime";
const version = process.argv[2];

if (!version) {
  console.error("usage: node scripts/assert-installed-dev.js <version>");
  process.exit(2);
}

const extensionDir = path.join(os.homedir(), ".vscode", "extensions", `${extensionID}-${version}`);
const checks = [
  ["package.json", "file"],
  ["dist/extension.js", "file"],
  ["dist/chat/SessionTreeProvider.js", "file"],
  ["dist/config/RuntimeControlsView.js", "file"],
  ["dist/approval/nativeTool.js", "file"],
  ["bin/contenox", "file"],
];

for (const [entry] of checks) {
  const file = path.join(extensionDir, entry);
  if (!fs.existsSync(file)) {
    fail(`installed extension is missing ${entry}: ${file}`);
  }
}

const pkg = readJson(path.join(extensionDir, "package.json"));
if (`${pkg.publisher}.${pkg.name}` !== extensionID) {
  fail(`installed extension id is ${pkg.publisher}.${pkg.name}, expected ${extensionID}`);
}
if (pkg.version !== version) {
  fail(`installed extension version is ${pkg.version}, expected ${version}`);
}
if (!pkg.contributes?.views?.contenox?.some((view) => view.id === "contenox.controls")) {
  fail("installed package does not contribute the Contenox Runtime controls view");
}
if (!pkg.contributes?.views?.contenox?.some((view) => view.id === "contenox.sessions")) {
  fail("installed package does not contribute the Contenox Sessions view");
}

const sessionTree = readText(path.join(extensionDir, "dist/chat/SessionTreeProvider.js"));
for (const marker of ["Provider", "Model", "Thinking", "HITL Policy", "contenox.selectHitlPolicy"]) {
  if (!sessionTree.includes(marker)) {
    fail(`installed SessionTreeProvider.js is missing marker ${JSON.stringify(marker)}`);
  }
}
if (fs.existsSync(path.join(extensionDir, "dist/chat/ChatPanel.js"))) {
  fail("installed extension still contains stale dist/chat/ChatPanel.js");
}

const approvalTool = readText(path.join(extensionDir, "dist/approval/nativeTool.js"));
for (const marker of ["approval.native.payload", "approval.native.payload.missing_details"]) {
  if (!approvalTool.includes(marker)) {
    fail(`installed nativeTool.js is missing marker ${JSON.stringify(marker)}`);
  }
}
if (approvalTool.includes("Contenox requests permission:")) {
  fail("installed extension still contains legacy notification approval text");
}

console.log(`Installed extension verified: ${extensionDir}`);

function readJson(file) {
  return JSON.parse(readText(file));
}

function readText(file) {
  return fs.readFileSync(file, "utf8");
}

function fail(message) {
  console.error(message);
  process.exit(1);
}
