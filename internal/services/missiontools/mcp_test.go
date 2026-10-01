package missiontools_test

import (
	"context"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/services/missionservice"
	"github.com/contenox/contenox/internal/services/missiontools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestUnit_MissionMCP(t *testing.T) {
	ctx, svc, id := setup(t)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	other := &missionservice.Mission{Intent: "other", AgentName: "other", HITLPolicyName: "default"}
	require.NoError(t, svc.Create(ctx, other))
	desc, close, err := missiontools.OpenMCP(ctx, missiontools.New(svc), id, t.TempDir(), []string{"contenox", "mission", "bridge"})
	require.NoError(t, err)
	t.Cleanup(close)
	path := desc.Args[len(desc.Args)-1]
	conn, err := net.Dial("unix", path)
	require.NoError(t, err)
	_, err = io.WriteString(conn, desc.Env[0].Value)
	require.NoError(t, err)
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: conn, Writer: conn}, nil)
	require.NoError(t, err)
	defer session.Close()
	listed, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	var names []string
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
	}
	require.ElementsMatch(t, []string{"mission_report", "mission_plan", "mission_ask_attention", "mission_finish"}, names)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "mission_report", Arguments: map[string]any{"kind": "finding", "summary": "actual finding", "missionId": other.ID}})
	require.NoError(t, err)
	require.False(t, result.IsError)
	reports, err := svc.ListReports(ctx, id, 10)
	require.NoError(t, err)
	require.Len(t, reports, 1)
	otherReports, err := svc.ListReports(ctx, other.ID, 10)
	require.NoError(t, err)
	require.Empty(t, otherReports)
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "mission_finish", Arguments: map[string]any{"status": "landed", "reason": "review complete"}})
	require.NoError(t, err)
	require.False(t, result.IsError)
	mission, err := svc.Get(ctx, id)
	require.NoError(t, err)
	require.Equal(t, missionservice.StatusLanded, mission.Status)
	close()
	_, err = os.Stat(path)
	require.True(t, os.IsNotExist(err))
}

func TestUnit_MissionMCPRejectsWrongCapability(t *testing.T) {
	ctx, svc, id := setup(t)
	desc, close, err := missiontools.OpenMCP(ctx, missiontools.New(svc), id, t.TempDir(), []string{"contenox"})
	require.NoError(t, err)
	defer close()
	conn, err := net.Dial("unix", desc.Args[0])
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetDeadline(time.Now().Add(time.Second)))
	_, err = io.WriteString(conn, strings.Repeat("0", 64))
	require.NoError(t, err)
	_, err = conn.Read(make([]byte, 1))
	require.Error(t, err)
	if timeout, ok := err.(net.Error); ok {
		require.False(t, timeout.Timeout(), "unauthorized connection must be closed immediately")
	}
}
