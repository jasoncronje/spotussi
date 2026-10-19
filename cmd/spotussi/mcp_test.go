package main

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"spotussi/internal/auth"
	"spotussi/internal/config"
	"spotussi/internal/db"
	"spotussi/internal/keyring"
	"spotussi/internal/mcp"
	"spotussi/internal/mock"
	"spotussi/internal/refdata"
)

// TestMain lets the test binary act as the spotussi binary for subprocess
// tests.
func TestMain(m *testing.M) {
	if os.Getenv("SPOTUSSI_TEST_MAIN") == "1" {
		os.Args = append([]string{"spotussi"}, strings.Fields(os.Getenv("SPOTUSSI_TEST_ARGS"))...)
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// TestMCPCommand runs `spotussi mcp` as a child process speaking stdio, the
// way Claude Desktop launches it, against /mcp on a mock server.
func TestMCPCommand(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	keys, _ := keyring.New(bytes.Repeat([]byte{3}, 32))
	a := auth.New(auth.Options{DB: d, Keyring: keys, Mode: config.AuthPassword})
	_, secret, err := a.CreateToken(ctx, "desktop")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	be := mock.NewWithClock(refdata.Generate(refdata.Config{Size: refdata.Tiny, Seed: 1, End: now, Timezone: "UTC"}),
		func() time.Time { return now })
	srv := httptest.NewServer(mcp.Handler(mcp.Options{Backend: be, Auth: a, Mode: "mock"}))
	defer srv.Close()

	cmd := exec.Command(os.Args[0])
	// The token comes from the environment, as the docs recommend.
	cmd.Env = append(os.Environ(), "SPOTUSSI_TEST_MAIN=1", "SPOTUSSI_TEST_ARGS=mcp --url "+srv.URL, "SPOTUSSI_TOKEN="+secret)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "test"}, nil).Connect(cctx, &sdk.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connect: %v (stderr: %s)", err, stderr.String())
	}
	tools, err := cs.ListTools(cctx, nil)
	if err != nil || len(tools.Tools) != 8 {
		t.Fatalf("list tools: %v %v", err, tools)
	}
	res, err := cs.CallTool(cctx, &sdk.CallToolParams{Name: "get_overview", Arguments: map[string]any{"range": "all"}})
	if err != nil || res.IsError || !strings.Contains(res.Content[0].(*sdk.TextContent).Text, `"top_artists"`) {
		t.Fatalf("get_overview: %v %+v", err, res)
	}
	if err := cs.Close(); err != nil {
		t.Errorf("close: %v (stderr: %s)", err, stderr.String())
	}
}

func TestMCPCommandUsage(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "SPOTUSSI_TEST_MAIN=1", "SPOTUSSI_TEST_ARGS=mcp", "SPOTUSSI_URL=", "SPOTUSSI_TOKEN=")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "usage: spotussi mcp") {
		t.Errorf("no URL or token: %v %s", err, out)
	}
}
