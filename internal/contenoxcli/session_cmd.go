// session_cmd.go — contenox session subcommand tree (new, list, switch, delete, show).
// Each subcommand opens only the DB; no LLM stack is needed.
package contenoxcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/libtracker"
	"github.com/contenox/contenox/messagestore"
	"github.com/contenox/contenox/runtimetypes"
	"github.com/contenox/contenox/taskengine"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

// sessionCmd is the parent "contenox session" command.
var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage chat sessions (new, list, switch, delete, show).",
	Long: `Create and switch named chat sessions.
Each session maintains its own persistent conversation history.

  contenox session new [name]     create a session and make it active
  contenox session list           list all sessions (* = active)
  contenox session switch <name>  switch the active session
  contenox session delete <name>  delete a session and its messages
  contenox session show           print the active session's conversation`,
	SilenceUsage: true,
}

var sessionNewCmd = &cobra.Command{
	Use:   "new [name]",
	Short: "Create a new session and make it active.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runSessionNew,
}

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all sessions (* = active).",
	Args:  cobra.NoArgs,
	RunE:  runSessionList,
}

var sessionSwitchCmd = &cobra.Command{
	Use:   "switch <name>",
	Short: "Switch the active session by name.",
	Args:  cobra.ExactArgs(1),
	RunE:  runSessionSwitch,
}

var sessionDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a session and all its messages.",
	Args:  cobra.ExactArgs(1),
	RunE:  runSessionDelete,
}

var sessionShowCmd = &cobra.Command{
	Use:   "show [name]",
	Short: "Print a session's conversation (default: active session).",
	Long: `Print the full conversation history for a session.

Defaults to the currently active session. Pass a session name to view another.

Flags:
  --tail N    Show only the last N messages
  --head N    Show only the first N messages

Examples:
  contenox session show
  contenox session show my-session
  contenox session show --tail 10`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSessionShow,
}

func init() {
	sessionShowCmd.Flags().Int("tail", 0, "Show last N messages (0 = all)")
	sessionShowCmd.Flags().Int("head", 0, "Show first N messages (0 = all)")
	sessionCmd.AddCommand(sessionNewCmd, sessionListCmd, sessionSwitchCmd, sessionDeleteCmd, sessionShowCmd)
}

// openSessionDB resolves the DB path from flags and opens SQLite.
func openSessionDB(cmd *cobra.Command) (context.Context, libdb.DBManager, func(), error) {
	dbPath, err := resolveDBPath(cmd)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid database path: %w", err)
	}
	ctx := libtracker.WithNewRequestID(context.Background())
	db, err := openDBAt(ctx, dbPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to open database: %w", err)
	}
	cleanup := func() {
		if err := db.Close(); err != nil {
			slog.Error("Error closing database", "error", err)
		}
	}
	return ctx, db, cleanup, nil
}

func runSessionNew(cmd *cobra.Command, args []string) error {
	ctx, db, cleanup, err := openSessionDB(cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	name := fmt.Sprintf("session-%s", uuid.New().String()[:8])
	if len(args) > 0 && args[0] != "" {
		name = args[0]
	}

	// Check name not already taken.
	store := messagestore.New(db.WithoutTransaction())
	if _, err := store.GetSessionByName(ctx, localIdentity, name); err == nil {
		return fmt.Errorf("session %q already exists; pick a different name", name)
	}

	newID := uuid.New().String()
	txExec, commit, release, txErr := db.WithTransaction(ctx)
	if txErr != nil {
		return fmt.Errorf("failed to start transaction: %w", txErr)
	}
	defer release()

	if err := messagestore.New(txExec).CreateNamedMessageIndex(ctx, newID, localIdentity, name); err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	if err := setActiveSessionID(ctx, txExec, newID); err != nil {
		return fmt.Errorf("failed to set active session: %w", err)
	}
	if err := commit(ctx); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	fmt.Printf("Created session %q. Now active.\n", name)
	return nil
}

func runSessionList(cmd *cobra.Command, _ []string) error {
	ctx, db, cleanup, err := openSessionDB(cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	exec := db.WithoutTransaction()
	sessions, err := messagestore.New(exec).ListAllSessions(ctx, localIdentity)
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}
	if len(sessions) == 0 {
		fmt.Println("No sessions yet. Run: contenox session new")
		return nil
	}

	activeID, _ := getActiveSessionID(ctx, exec)
	store := messagestore.New(exec)
	for _, s := range sessions {
		prefix := "  "
		if s.ID == activeID {
			prefix = "* "
		}
		displayName := s.Name
		if displayName == "" {
			displayName = s.ID[:8] + "…"
		}
		count, _ := store.CountMessages(ctx, s.ID)
		fmt.Printf("%s%-24s (%d messages)\n", prefix, displayName, count)
	}
	return nil
}

func runSessionSwitch(cmd *cobra.Command, args []string) error {
	ctx, db, cleanup, err := openSessionDB(cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	name := args[0]
	exec := db.WithoutTransaction()
	si, err := messagestore.New(exec).GetSessionByName(ctx, localIdentity, name)
	if err != nil {
		if errors.Is(err, messagestore.ErrNotFound) {
			return fmt.Errorf("session %q not found; run 'contenox session list' to see available sessions", name)
		}
		return fmt.Errorf("failed to look up session: %w", err)
	}

	if err := setActiveSessionID(ctx, exec, si.ID); err != nil {
		return fmt.Errorf("failed to switch session: %w", err)
	}
	fmt.Printf("Switched to session %q.\n", name)
	return nil
}

func runSessionDelete(cmd *cobra.Command, args []string) error {
	ctx, db, cleanup, err := openSessionDB(cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	name := args[0]
	exec := db.WithoutTransaction()
	si, err := messagestore.New(exec).GetSessionByName(ctx, localIdentity, name)
	if err != nil {
		if errors.Is(err, messagestore.ErrNotFound) {
			return fmt.Errorf("session %q not found", name)
		}
		return fmt.Errorf("failed to look up session: %w", err)
	}

	txExec, commit, release, txErr := db.WithTransaction(ctx)
	if txErr != nil {
		return fmt.Errorf("failed to start transaction: %w", txErr)
	}
	defer release()

	// ON DELETE CASCADE removes messages; we only need to remove the index.
	if err := messagestore.New(txExec).DeleteMessageIndex(ctx, si.ID, localIdentity); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	// If this was the active session, clear the pointer.
	activeID, _ := getActiveSessionID(ctx, txExec)
	if activeID == si.ID {
		raw, _ := marshalJSON("")
		runtimetypes.New(txExec).SetKV(ctx, kvActiveSession, raw) //nolint:errcheck
		fmt.Printf("Deleted session %q (was active; run 'contenox session new' or 'contenox session switch' to set a new active session).\n", name)
	} else {
		fmt.Printf("Deleted session %q.\n", name)
	}

	return commit(ctx)
}

func runSessionShow(cmd *cobra.Command, args []string) error {
	ctx, db, cleanup, err := openSessionDB(cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	tailN, _ := cmd.Flags().GetInt("tail")
	headN, _ := cmd.Flags().GetInt("head")

	exec := db.WithoutTransaction()
	store := messagestore.New(exec)
	sessions, _ := store.ListAllSessions(ctx, localIdentity)

	var sessionID, sessionName string

	if len(args) > 0 {
		// Look up by name.
		name := args[0]
		for _, s := range sessions {
			if s.Name == name {
				sessionID = s.ID
				sessionName = s.Name
				break
			}
		}
		if sessionID == "" {
			return fmt.Errorf("session %q not found; run 'contenox session list'", name)
		}
	} else {
		// Use active session.
		activeID, err := getActiveSessionID(ctx, exec)
		if err != nil || activeID == "" {
			return fmt.Errorf("no active session; run 'contenox session new' to create one")
		}
		sessionID = activeID
		sessionName = sessionID[:8] + "…"
		for _, s := range sessions {
			if s.ID == sessionID && s.Name != "" {
				sessionName = s.Name
				break
			}
		}
	}

	rawMsgs, err := store.ListMessages(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to read messages: %w", err)
	}
	if len(rawMsgs) == 0 {
		fmt.Printf("Session %q has no messages yet.\n", sessionName)
		return nil
	}

	// Apply head/tail filters.
	slice := rawMsgs
	if headN > 0 && headN < len(slice) {
		slice = slice[:headN]
	} else if tailN > 0 && tailN < len(slice) {
		slice = slice[len(slice)-tailN:]
	}

	fmt.Printf("━━━━ Session: %s (%d/%d messages) ━━━━\n", sessionName, len(slice), len(rawMsgs))
	for _, raw := range slice {
		var m taskengine.Message
		if err := json.Unmarshal(raw.Payload, &m); err != nil {
			continue
		}
		ts := ""
		if !m.Timestamp.IsZero() {
			ts = m.Timestamp.Format(time.RFC3339)
		}
		if ts != "" {
			fmt.Printf("[%s] %s:\n", ts, m.Role)
		} else {
			fmt.Printf("%s:\n", m.Role)
		}
		fmt.Printf("  %s\n\n", m.Content)
	}
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━\n")
	return nil
}
