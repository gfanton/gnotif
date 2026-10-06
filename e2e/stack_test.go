//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// devtestMnemonic is integration.DefaultAccount_Seed in gno v1.5.0, the
// account gnodev premines and deploys with by default.
const devtestMnemonic = "source bonus chronic canvas draft south burst lottery vacant surface solve popular case indicate oppose farm nothing bullet exhibit title speed wink action roast"

const pingpongPath = "gno.land/r/dev/pingpong/v0"

// gnodev outside a workspace does not resolve imports from its examples, so
// the realms' dependencies are loaded from the gno clone explicitly,
// dependencies before dependents.
var examplePackages = []string{"avl/v0", "cford32/v0", "seqid/v0", "markdown/sanitize/v0"}

// stack is a local chain: gnodev with the registry loaded and pingpong
// deployed, and a tx-indexer reading it.
type stack struct {
	t       *testing.T
	tools   string
	keybase string
	rpc     string // host:port of gnodev's RPC
	indexer string // tx-indexer GraphQL URL
	addrs   map[string]string
}

func newStack(t *testing.T) *stack {
	t.Helper()
	tools, root := os.Getenv("GNOTIF_TOOLS"), os.Getenv("GNOTIF_ROOT")
	require.NotEmpty(t, tools, "GNOTIF_TOOLS is unset; run make e2e")
	require.NotEmpty(t, root, "GNOTIF_ROOT is unset; run make e2e")
	s := &stack{t: t, tools: tools, keybase: t.TempDir(), addrs: map[string]string{}}

	s.gnokey(devtestMnemonic+"\n\n\n", "add", "devtest", "-recover", "-insecure-password-stdin", "-home", s.keybase)
	s.gnokey("\n\n", "add", "player2", "-insecure-password-stdin", "-home", s.keybase)
	for _, m := range regexp.MustCompile(`(\w+) \(local\) - addr: (g1[0-9a-z]+)`).FindAllStringSubmatch(s.gnokey("", "list", "-home", s.keybase), -1) {
		s.addrs[m[1]] = m[2]
	}
	require.Contains(t, s.addrs, "devtest")
	require.Contains(t, s.addrs, "player2")

	s.rpc = freeAddr(t)
	args := []string{"local", "-no-web", "-no-watch", "-home", s.keybase, "-node-rpc-listener", s.rpc}
	for _, p := range examplePackages {
		args = append(args, filepath.Join(tools, "gno-src", "examples", "gno.land", "p", "nt", p))
	}
	args = append(args, filepath.Join(root, "gno", "r", "gnotif", "v0"))
	s.start("gnodev", []string{"GNOROOT=" + filepath.Join(tools, "gno-src")}, args...)
	require.Eventually(t, func() bool {
		resp, err := http.Get("http://" + s.rpc + "/status")
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 2*time.Minute, 250*time.Millisecond, "gnodev RPC never answered")
	// The tx-indexer files genesis transactions at height 0, below where
	// gnotifd reads, so pingpong declares its trigger in a deploy transaction.
	s.addpkg("devtest", filepath.Join(root, "demo", "gno.land", "r", "pingpong", "v0"), pingpongPath)

	idx := freeAddr(t)
	s.indexer = "http://" + idx + "/graphql/query"
	s.start("tx-indexer", nil, "start", "-remote", "http://"+s.rpc, "-db-path", filepath.Join(t.TempDir(), "indexer"), "-listen-address", idx)
	require.Eventually(t, func() bool { return s.latestHeight() >= 1 }, time.Minute, 250*time.Millisecond, "tx-indexer never indexed a block")
	return s
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return addr
}

// start runs a tool until the test ends. Its output goes to a log file,
// printed when the test fails.
func (s *stack) start(name string, env []string, args ...string) {
	t := s.t
	t.Helper()
	logPath := filepath.Join(t.TempDir(), name+".log")
	out, err := os.Create(logPath)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, filepath.Join(s.tools, name), args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Dir = t.TempDir()
	cmd.Stdout, cmd.Stderr = out, out
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 5 * time.Second
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		cancel()
		cmd.Wait()
		out.Close()
		if t.Failed() {
			b, _ := os.ReadFile(logPath)
			t.Logf("%s output (last 4 KB):\n%s", name, b[max(0, len(b)-4096):])
		}
	})
}

func (s *stack) gnokey(stdin string, args ...string) string {
	s.t.Helper()
	cmd := exec.Command(filepath.Join(s.tools, "gnokey"), args...)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	require.NoError(s.t, err, "gnokey %s:\n%s", strings.Join(args, " "), out)
	return string(out)
}

// call sends a MsgCall to pingpong signed by key and returns gnokey's output.
func (s *stack) call(key, fn string, args ...string) string {
	s.t.Helper()
	return s.callPackage(key, pingpongPath, fn, args...)
}

// callPackage sends a MsgCall to the package at pkgPath signed by key and
// returns gnokey's output.
func (s *stack) callPackage(key, pkgPath, fn string, args ...string) string {
	s.t.Helper()
	a := []string{"maketx", "call", "-pkgpath", pkgPath, "-func", fn}
	for _, arg := range args {
		a = append(a, "-args", arg)
	}
	a = append(a, "-gas-fee", "1000000ugnot", "-gas-wanted", "50000000", "-broadcast",
		"-chainid", "dev", "-remote", s.rpc, "-insecure-password-stdin", "-home", s.keybase, key)
	return s.gnokey("\n", a...)
}

var (
	heightLine = regexp.MustCompile(`HEIGHT:\s+(\d+)`)
	hashLine   = regexp.MustCompile(`TX HASH:\s+(\S+)`)
)

// committed returns the block height and the base64 hash gnokey printed for
// a broadcast transaction.
func committed(t *testing.T, out string) (int64, string) {
	t.Helper()
	height, hash := heightLine.FindStringSubmatch(out), hashLine.FindStringSubmatch(out)
	require.NotNil(t, height, "no HEIGHT in gnokey output:\n%s", out)
	require.NotNil(t, hash, "no TX HASH in gnokey output:\n%s", out)
	h, err := strconv.ParseInt(height[1], 10, 64)
	require.NoError(t, err)
	return h, hash[1]
}

// addpkg deploys the package in dir at pkgPath, without its tests, signed
// by key.
func (s *stack) addpkg(key, dir, pkgPath string) {
	t := s.t
	t.Helper()
	pkg := t.TempDir()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		name := e.Name()
		deployed := name == "gnomod.toml" || strings.HasSuffix(name, ".gno") && !strings.HasSuffix(name, "_test.gno")
		if !e.Type().IsRegular() || !deployed {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(pkg, name), b, 0o644))
	}
	s.gnokey("\n", "maketx", "addpkg", "-pkgpath", pkgPath, "-pkgdir", pkg, "-max-deposit", "10000000ugnot",
		"-gas-fee", "1000000ugnot", "-gas-wanted", "50000000", "-broadcast",
		"-chainid", "dev", "-remote", s.rpc, "-insecure-password-stdin", "-home", s.keybase, key)
}

func (s *stack) latestHeight() int64 {
	resp, err := http.Post(s.indexer, "application/json", strings.NewReader(`{"query":"{ latestBlockHeight }"}`))
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	var out struct {
		Data struct {
			LatestBlockHeight int64 `json:"latestBlockHeight"`
		} `json:"data"`
	}
	if json.NewDecoder(resp.Body).Decode(&out) != nil {
		return 0
	}
	return out.Data.LatestBlockHeight
}
