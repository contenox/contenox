#!/usr/bin/env node

const { spawnSync } = require("node:child_process");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { targetFromEnv } = require("./vscode-targets");

const extensionRoot = path.resolve(__dirname, "..");
const packagePath = path.join(extensionRoot, "package.json");
const pkg = readJson(packagePath);
const target = targetFromEnv();
const output = path.join(extensionRoot, "artifacts", `${pkg.name}-${target.name}-${pkg.version}-proposed.vsix`);
const stageRoot = fs.mkdtempSync(path.join(os.tmpdir(), "contenox-vscode-proposed-"));
const vsce = path.join(extensionRoot, "node_modules", ".bin", process.platform === "win32" ? "vsce.cmd" : "vsce");
const chatSessionType = "contenox-agent";

for (const entry of [
  "package.json",
  ".vscodeignore",
  "README.md",
  "CHANGELOG.md",
  "SECURITY.md",
  "SUPPORT.md",
  "LICENSE",
  "dist",
  "media",
  "walkthrough",
  "bin",
]) {
  copyRequired(entry);
}

const stagedPackagePath = path.join(stageRoot, "package.json");
const staged = readJson(stagedPackagePath);
staged.enabledApiProposals = unique([
  ...(staged.enabledApiProposals || []),
  "chatParticipantPrivate",
  "chatSessionsProvider",
  "languageModelProxy",
]);
staged.activationEvents = unique([
  ...(staged.activationEvents || []),
  `onChatSession:${chatSessionType}`,
  `onChatParticipant:${chatSessionType}`,
]);
staged.contributes = staged.contributes || {};
staged.contributes.chatSessions = [
  {
    type: chatSessionType,
    name: "contenox",
    displayName: "Contenox",
    description: "Use the local Contenox runtime as a VS Code coding agent session.",
    icon: "media/contenox-activity-dark.png",
    canDelegate: true,
    inputPlaceholder: "Ask Contenox to work in this workspace",
    welcomeTitle: "Contenox",
    welcomeMessage: "Local Contenox runtime session. Requests run through the bundled Contenox runtime process.",
    commands: chatParticipantCommands(staged),
  },
];
staged.scripts = {};
writeJson(stagedPackagePath, staged);

fs.rmSync(output, { force: true });
fs.mkdirSync(path.dirname(output), { recursive: true });

const vsceArgs = [
  "package",
  "--target",
  target.name,
  "--no-dependencies",
  "--no-yarn",
  "--out",
  output,
];
if (process.env.CONTENOX_VSCODE_SKIP_VSCE_SECRET_SCAN === "1") {
  vsceArgs.splice(vsceArgs.indexOf("--out"), 0, "--allow-package-all-secrets", "--allow-package-env-file");
}

const result = spawnSync(vsce, vsceArgs, {
  cwd: stageRoot,
  stdio: "inherit",
});

fs.rmSync(stageRoot, { recursive: true, force: true });

if (result.status !== 0) {
  process.exit(result.status ?? 1);
}

console.log(`Built proposed ${target.name} VS Code extension: ${path.relative(extensionRoot, output)}`);

function copyRequired(entry) {
  const src = path.join(extensionRoot, entry);
  const dst = path.join(stageRoot, entry);
  if (!fs.existsSync(src)) {
    throw new Error(`required package input is missing: ${entry}`);
  }
  fs.cpSync(src, dst, {
    recursive: true,
    filter: (source) => !source.includes(`${path.sep}.git${path.sep}`),
  });
}

function chatParticipantCommands(manifest) {
  const participants = manifest.contributes?.chatParticipants;
  if (!Array.isArray(participants)) {
    return [];
  }
  const participant = participants.find((candidate) => candidate.id === "contenox") || participants[0];
  return Array.isArray(participant?.commands) ? participant.commands : [];
}

function unique(values) {
  return Array.from(new Set(values));
}

function readJson(file) {
  return JSON.parse(fs.readFileSync(file, "utf8"));
}

function writeJson(file, value) {
  fs.writeFileSync(file, `${JSON.stringify(value, null, 2)}\n`);
}
