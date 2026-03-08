package contenoxcli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/libtracker"
	"github.com/contenox/contenox/runtimetypes"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Manage MCP servers (add, list, show, remove).",
	Long: `Register and manage Model Context Protocol (MCP) servers.

MCP servers extend the runtime with additional tools and resources callable by the model.
Two transport modes are supported:

  stdio   Spawn a local process and communicate via stdin/stdout.
          Requires --command (and optionally --args).

  sse     Connect to a remote MCP server via Server-Sent Events.
          Requires --url.

  http    Connect to a remote MCP server via HTTP streaming.
          Requires --url.

Examples:
  # Register a local stdio MCP server:
  contenox mcp add myserver --transport stdio --command npx --args "-y,@modelcontextprotocol/server-filesystem,/tmp"

  # Register a remote SSE-based MCP server:
  contenox mcp add remote --transport sse --url https://mcp.example.com/sse

  contenox mcp list
  contenox mcp show myserver
  contenox mcp remove myserver`,
}

var mcpAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Register an MCP server.",
	Long: `Register a named MCP server in the local SQLite database.

Transport types:
  stdio   Spawn a local command (--command required; --args optional)
  sse     Connect to a remote server via Server-Sent Events (--url required)
  http    Connect to a remote server via HTTP streaming (--url required)

For authentication, use:
  --auth-type bearer  with --auth-token <token>  or  --auth-env <ENV_VAR>

Examples:
  # Stdio: spawn a local filesystem MCP server
  contenox mcp add fs --transport stdio \
    --command npx --args "-y,@modelcontextprotocol/server-filesystem,/tmp"

  # SSE: connect to a remote MCP endpoint
  contenox mcp add remote --transport sse --url https://mcp.example.com/sse \
    --auth-type bearer --auth-env MCP_TOKEN`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		name := args[0]
		flags := cmd.Flags()

		transport, _ := flags.GetString("transport")
		command, _ := flags.GetString("command")
		cmdArgs, _ := flags.GetStringSlice("args")
		url, _ := flags.GetString("url")
		timeout, _ := flags.GetInt("timeout")

		authType, _ := flags.GetString("auth-type")
		authToken, _ := flags.GetString("auth-token")
		authEnv, _ := flags.GetString("auth-env")

		if transport == "" {
			return fmt.Errorf("--transport is required (stdio, sse, http)")
		}
		if transport == "stdio" && command == "" {
			return fmt.Errorf("--command is required for stdio transport")
		}
		if (transport == "sse" || transport == "http") && url == "" {
			return fmt.Errorf("--url is required for sse/http transport")
		}

		db, store, err := openMCPDB(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		srv := &runtimetypes.MCPServer{
			Name:                  name,
			Transport:             transport,
			Command:               command,
			Args:                  cmdArgs,
			URL:                   url,
			ConnectTimeoutSeconds: timeout,
			AuthType:              authType,
			AuthToken:             authToken,
			AuthEnvKey:            authEnv,
		}

		if err := store.CreateMCPServer(ctx, srv); err != nil {
			return fmt.Errorf("failed to add MCP server: %w", err)
		}
		fmt.Printf("MCP server %q added successfully.\n", name)
		return nil
	},
}

var mcpListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered MCP servers.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		db, store, err := openMCPDB(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		servers, err := store.ListMCPServers(ctx, nil, 100)
		if err != nil {
			return fmt.Errorf("failed to list MCP servers: %w", err)
		}

		if len(servers) == 0 {
			fmt.Println("No MCP servers registered. Run: contenox mcp add <name> --transport <type> ...")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tTRANSPORT\tCOMMAND/URL")
		for _, s := range servers {
			target := s.Command
			if s.Transport == "sse" || s.Transport == "http" {
				target = s.URL
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", s.Name, s.Transport, target)
		}
		return w.Flush()
	},
}

var mcpShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show details for an MCP server.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		name := args[0]
		db, store, err := openMCPDB(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		srv, err := store.GetMCPServerByName(ctx, name)
		if err != nil {
			return fmt.Errorf("mcp server %q not found: %w", name, err)
		}

		b, _ := json.MarshalIndent(srv, "", "  ")
		fmt.Println(string(b))
		return nil
	},
}

var mcpRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm"},
	Short:   "Remove a registered MCP server.",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		name := args[0]
		db, store, err := openMCPDB(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		srv, err := store.GetMCPServerByName(ctx, name)
		if err != nil {
			return fmt.Errorf("mcp server %q not found: %w", name, err)
		}

		if err := store.DeleteMCPServer(ctx, srv.ID); err != nil {
			return fmt.Errorf("failed to remove mcp server: %w", err)
		}
		fmt.Printf("MCP server %q removed.\n", name)
		return nil
	},
}

func openMCPDB(cmd *cobra.Command) (libdbexec.DBManager, runtimetypes.Store, error) {
	dbPath, err := resolveDBPath(cmd)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid database path: %w", err)
	}
	dbCtx := libtracker.WithNewRequestID(context.Background())
	db, err := openDBAt(dbCtx, dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open database: %w", err)
	}
	return db, runtimetypes.New(db.WithoutTransaction()), nil
}

func init() {
	mcpAddCmd.Flags().String("transport", "stdio", "Transport type: stdio (local process), sse, or http (remote server)")
	mcpAddCmd.Flags().String("command", "", "Command to execute (required for stdio transport)")
	mcpAddCmd.Flags().StringSlice("args", nil, "Arguments for the command, comma-separated (for stdio transport)")
	mcpAddCmd.Flags().String("url", "", "URL of the remote MCP server (required for sse/http transport)")
	mcpAddCmd.Flags().Int("timeout", 0, "Connection timeout in seconds (0 = no timeout)")

	mcpAddCmd.Flags().String("auth-type", "", "Authentication type (e.g. bearer)")
	mcpAddCmd.Flags().String("auth-token", "", "Authentication token literal (prefer --auth-env)")
	mcpAddCmd.Flags().String("auth-env", "", "Environment variable containing the authentication token")

	mcpCmd.AddCommand(mcpAddCmd)
	mcpCmd.AddCommand(mcpListCmd)
	mcpCmd.AddCommand(mcpShowCmd)
	mcpCmd.AddCommand(mcpRemoveCmd)
}
