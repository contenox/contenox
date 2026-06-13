import * as assert from "node:assert/strict";
import * as fs from "node:fs";
import * as path from "node:path";
import { PassThrough } from "node:stream";
import * as vscode from "vscode";
import { approvalToolName } from "../approval/nativeTool";
import { BridgeClient } from "../bridge/BridgeClient";
import { JsonRpcFramer } from "../bridge/JsonRpcFramer";
import type { RequestPermissionParams } from "../bridge/protocol";
import { approvalEventFromPermissionRequest } from "../chat/participant";
import { ContenoxOutput } from "../logging/output";
import { TelemetryLogger } from "../logging/telemetry";

suite("Contenox VS Code extension", () => {
  test("activates and registers core commands", async () => {
    const extension = vscode.extensions.getExtension("contenox.runtime");
    assert.ok(extension, "extension should be available in the Extension Development Host");
    await extension.activate();

    const commands = await vscode.commands.getCommands(true);
    for (const command of [
      "contenox.openWalkthrough",
      "contenox.openChat",
      "contenox.runSetup",
      "contenox.showStatus",
      "contenox.testAutocompleteAtCursor",
    ]) {
      assert.ok(commands.includes(command), `${command} should be registered`);
    }

    assert.ok(vscode.lm.tools.some((tool) => tool.name === approvalToolName), `${approvalToolName} should be registered`);
  });

  test("manifest keeps command titles, walkthroughs, and welcome content coherent", () => {
    const manifest = readManifest();
    const contributes = manifest.contributes;

    assert.deepEqual(manifest.extensionKind, ["workspace"]);
    assert.ok(contributes.views?.contenox?.some((entry) => entry.id === "contenox.controls"));
    assert.ok(contributes.viewsWelcome?.some((entry) => entry.view === "contenox.controls"));
    assert.ok(contributes.viewsWelcome?.some((entry) => entry.view === "contenox.sessions"));

    const walkthrough = contributes.walkthroughs?.find((entry) => entry.id === "getStarted");
    assert.ok(walkthrough, "getStarted walkthrough should be contributed");
    assert.equal(walkthrough.steps.length, 5);

    const commandTitles = new Map(contributes.commands.map((command) => [command.command, command.title]));
    assert.equal(commandTitles.get("contenox.openWalkthrough"), "Open Walkthrough");
    for (const [command, title] of commandTitles) {
      assert.ok(!title.startsWith("Contenox:"), `${command} should not duplicate the Contenox category prefix`);
    }
    for (const command of ["contenox.selectProvider", "contenox.selectChatModel", "contenox.selectHitlPolicy", "contenox.selectThinkLevel"]) {
      assert.ok(commandTitles.has(command), `${command} should be contributed for the sessions sidebar`);
    }

    const approvalTool = contributes.languageModelTools?.find((tool) => tool.name === approvalToolName);
    assert.ok(approvalTool, `${approvalToolName} should be contributed`);
    assert.equal(approvalTool.displayName, "Approve Contenox Tool Call");
    assert.deepEqual(approvalTool.inputSchema.required, ["approvalId", "title", "summary"]);

    const referencedMarkdown = walkthrough.steps.map((step) => step.media?.markdown).filter(isString);
    for (const relativePath of referencedMarkdown) {
      const absolutePath = path.join(extensionRoot(), relativePath);
      assert.ok(fs.existsSync(absolutePath), `${relativePath} should exist`);
    }
  });

  test("bridge answers ACP-shaped permission requests", async () => {
    const outbound = new PassThrough();
    const inbound = new PassThrough();
    const output = new ContenoxOutput();
    const telemetry = new TelemetryLogger("test");
    const client = new BridgeClient(outbound, inbound, output, 1000, false, telemetry);
    const responses: unknown[] = [];
    const responseFramer = new JsonRpcFramer(new PassThrough(), (message) => responses.push(message), (error) => {
      throw error;
    });
    outbound.on("data", (chunk: Buffer) => responseFramer.accept(chunk));

    const disposable = client.pushPermissionRequestHandler(async (params) => {
      assert.equal(params.sessionId, "session-1");
      assert.equal(params.toolCall.toolCallId, "call-1");
      return { outcome: { outcome: "selected", optionId: "allow" } };
    });
    try {
      writeFramed(inbound, {
        jsonrpc: "2.0",
        id: 99,
        method: "session/request_permission",
        params: {
          sessionId: "session-1",
          toolCall: {
            toolCallId: "call-1",
            title: "local_shell.local_shell: python3",
            kind: "execute",
            status: "pending",
            rawInput: { command: "python3" },
          },
          options: [
            { optionId: "allow", name: "Allow", kind: "allow_once" },
            { optionId: "deny", name: "Deny", kind: "reject_once" },
          ],
        },
      });

      await eventually(() => responses.length === 1);
      assert.deepEqual(responses[0], {
        jsonrpc: "2.0",
        id: 99,
        result: { outcome: { outcome: "selected", optionId: "allow" } },
      });
    } finally {
      disposable.dispose();
      client.dispose();
      telemetry.dispose();
      output.dispose();
    }
  });

  test("permission request conversion preserves meaningful approval-card details", () => {
    const params: RequestPermissionParams = {
      sessionId: "session-1",
      toolCall: {
        toolCallId: "call-1",
        title: "local_fs.sed: Questions: hello@contenox.com in README.md",
        kind: "edit",
        status: "pending",
        rawInput: {
          path: "README.md",
          pattern: "Questions: hello@contenox.com",
          replacement: "Questions: **hello@contenox.com**",
        },
        content: [
          {
            type: "diff",
            path: "README.md",
            oldText: "Questions: hello@contenox.com\n",
            newText: "Questions: **hello@contenox.com**\n",
          },
        ],
        _meta: {
          toolsName: "local_fs",
          toolName: "sed",
          policyName: "hitl-policy-strict.json",
          policyPath: "/home/naro/.contenox/hitl-policy-strict.json",
          diff: "--- README.md\n+++ README.md\n@@\n-Questions: hello@contenox.com\n+Questions: **hello@contenox.com**\n",
        },
      },
      options: [
        { optionId: "allow", name: "Allow", kind: "allow_once" },
        { optionId: "deny", name: "Deny", kind: "reject_once" },
      ],
    };

    const event = approvalEventFromPermissionRequest(params);

    assert.deepEqual(event.args, {
      path: "README.md",
      pattern: "Questions: hello@contenox.com",
      replacement: "Questions: **hello@contenox.com**",
    });
    assert.equal(event.toolsName, "local_fs");
    assert.equal(event.toolName, "sed");
    assert.equal(event.policyName, "hitl-policy-strict.json");
    assert.equal(event.policyPath, "/home/naro/.contenox/hitl-policy-strict.json");
    assert.equal(event.diffOld, "Questions: hello@contenox.com\n");
    assert.equal(event.diffNew, "Questions: **hello@contenox.com**\n");
    assert.ok(event.diff?.includes("+Questions: **hello@contenox.com**"));
  });

  test("permission request conversion drops empty diff placeholders", () => {
    const event = approvalEventFromPermissionRequest({
      sessionId: "session-1",
      toolCall: {
        toolCallId: "call-blank",
        title: "local_fs.sed: README.md",
        rawInput: "{\"path\":\"README.md\",\"pattern\":\"x\",\"replacement\":\"y\"}",
        content: [{ type: "diff", path: "README.md", oldText: "", newText: "" }],
        _meta: {
          toolsName: "local_fs",
          toolName: "sed",
          diff: "",
          diffOld: "",
          diffNew: "",
        },
      },
      options: [
        { optionId: "allow", name: "Allow", kind: "allow_once" },
        { optionId: "deny", name: "Deny", kind: "reject_once" },
      ],
    });

    assert.deepEqual(event.args, { path: "README.md", pattern: "x", replacement: "y" });
    assert.equal(event.diff, undefined);
    assert.equal(event.diffOld, undefined);
    assert.equal(event.diffNew, undefined);
  });
});

interface ExtensionManifest {
  extensionKind?: string[];
  contributes: {
    commands: Array<{ command: string; title: string }>;
    languageModelTools?: Array<{
      name: string;
      displayName: string;
      inputSchema: {
        required: string[];
      };
    }>;
    viewsWelcome?: Array<{ view: string; contents: string }>;
    views?: {
      contenox?: Array<{ id: string; name: string }>;
    };
    walkthroughs?: Array<{
      id: string;
      steps: Array<{
        id: string;
        media?: {
          markdown?: string;
        };
      }>;
    }>;
  };
}

function readManifest(): ExtensionManifest {
  return JSON.parse(fs.readFileSync(path.join(extensionRoot(), "package.json"), "utf8")) as ExtensionManifest;
}

function extensionRoot(): string {
  return path.resolve(__dirname, "..", "..");
}

function isString(value: unknown): value is string {
  return typeof value === "string";
}

function writeFramed(stream: PassThrough, message: unknown): void {
  const payload = Buffer.from(JSON.stringify(message), "utf8");
  stream.write(Buffer.concat([Buffer.from(`Content-Length: ${payload.length}\r\n\r\n`, "ascii"), payload]));
}

async function eventually(predicate: () => boolean): Promise<void> {
  const deadline = Date.now() + 1000;
  while (Date.now() < deadline) {
    if (predicate()) {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  assert.ok(predicate(), "condition was not met before timeout");
}
