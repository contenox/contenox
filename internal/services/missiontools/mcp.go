package missiontools

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/contenox/contenox/internal/kernel/taskengine"
	"github.com/contenox/contenox/libacp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// BridgeTokenEnv carries the per-mission MCP capability to the stdio proxy.
const BridgeTokenEnv = "CONTENOX_MISSION_BRIDGE_TOKEN"

// OpenMCP exposes only the bound mission's tools over an authenticated local socket.
// command is the host executable's stdio proxy command, without the socket argument.
func OpenMCP(ctx context.Context, repo taskengine.ToolsRepo, missionID, cwd string, command []string) (libacp.McpServer, func(), error) {
	if missionID == "" || len(command) == 0 {
		return libacp.McpServer{}, nil, fmt.Errorf("mission MCP requires a mission and proxy command")
	}
	bound := WithWorkdir(WithMissionID(ctx, missionID), cwd)
	declared, err := repo.GetToolsForToolsByName(bound, ToolsProviderName)
	if err != nil {
		return libacp.McpServer{}, nil, err
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "contenox-mission", Version: "1"}, nil)
	for _, tool := range declared {
		f := tool.Function
		server.AddTool(&mcp.Tool{Name: f.Name, Description: f.Description, InputSchema: f.Parameters}, func(callCtx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var args map[string]any
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return nil, err
			}
			callCtx = WithWorkdir(WithMissionID(callCtx, missionID), cwd)
			value, kind, err := repo.Exec(callCtx, time.Now(), args, false, &taskengine.ToolsCall{Name: ToolsProviderName, ToolName: f.Name})
			if err != nil {
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
			}
			text := fmt.Sprint(value)
			if kind == taskengine.DataTypeJSON {
				raw, err := json.Marshal(value)
				if err != nil {
					return nil, err
				}
				text = string(raw)
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil
		})
	}
	dir, err := os.MkdirTemp("", "cnx-mission-")
	if err != nil {
		return libacp.McpServer{}, nil, err
	}
	path := filepath.Join(dir, "mcp.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		_ = os.Remove(dir)
		return libacp.McpServer{}, nil, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		_ = listener.Close()
		_ = os.Remove(dir)
		return libacp.McpServer{}, nil, err
	}
	token := hex.EncodeToString(secret)
	life, cancel := context.WithCancel(ctx)
	var once sync.Once
	close := func() {
		once.Do(func() {
			cancel()
			_ = listener.Close()
			_ = os.Remove(path)
			_ = os.Remove(dir)
		})
	}
	stop := context.AfterFunc(life, close)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go serveMissionMCP(life, server, conn, token)
		}
	}()
	return libacp.McpServer{Name: "contenox-mission", Command: command[0], Args: append(append([]string(nil), command[1:]...), path), Env: []libacp.EnvVariable{{Name: BridgeTokenEnv, Value: token}}}, func() { stop(); close() }, nil
}

func serveMissionMCP(ctx context.Context, server *mcp.Server, conn net.Conn, token string) {
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	provided := make([]byte, len(token))
	if _, err := io.ReadFull(conn, provided); err != nil || subtle.ConstantTimeCompare(provided, []byte(token)) != 1 {
		return
	}
	_ = conn.SetReadDeadline(time.Time{})
	session, err := server.Connect(ctx, &mcp.IOTransport{Reader: conn, Writer: conn}, nil)
	if err != nil {
		return
	}
	defer session.Close()
	_ = session.Wait()
}

// ProxyMCP connects an agent's stdio MCP transport to its mission-scoped host endpoint.
func ProxyMCP(ctx context.Context, path, token string, in io.Reader, out io.Writer) error {
	if len(token) != 64 {
		return fmt.Errorf("missing mission MCP capability")
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", path)
	if err != nil {
		return err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if _, err := io.WriteString(conn, token); err != nil {
		return err
	}
	go func() {
		_, _ = io.Copy(conn, in)
		_ = conn.Close()
	}()
	_, err = io.Copy(out, conn)
	return err
}
