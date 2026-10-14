package cliauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/shared/jsonkit"
)

func TestCredentialProcess(t *testing.T) {
	raw := os.Getenv("CLIAUTH_PROCESS_ORG")
	if raw == "" {
		return
	}
	org, err := cloudapi.ParseOrg(raw)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	authorize, err := (Manager{}).Authorize(ctx, org)
	if os.Getenv("CLIAUTH_EXPECT_PENDING") == "1" {
		if err == nil || !strings.Contains(err.Error(), "login") {
			t.Fatalf("expected login recovery: %v", err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	assertAuthorizationToken(t, authorize, org, "new-access")
}

func credentialChild(org cloudapi.Org, expectPending bool) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestCredentialProcess$", "-test.v") //nolint:gosec // G204: rerun this test binary as a competing CLI process
	cmd.Env = append(os.Environ(), "CLIAUTH_PROCESS_ORG="+org.String())
	if expectPending {
		cmd.Env = append(cmd.Env, "CLIAUTH_EXPECT_PENDING=1")
	}
	return cmd
}

func TestConcurrentProcessesRefreshOnce(t *testing.T) {
	t.Setenv("ANFRA_HOME", t.TempDir())
	var origin string
	var entry storedCredential
	var posts atomic.Int32
	ready := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-ready
		posts.Add(1)
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Method != "POST" || r.Form.Get("refresh_token") != "old-refresh" || r.Form.Get("resource") != origin+"/api" {
			t.Error("invalid refresh request")
		}
		var pending bool
		// Read without taking the lock held by the requesting child.
		raw, err := os.ReadFile(os.Getenv("ANFRA_HOME") + "/credentials.json") //nolint:gosec // G703: this test's isolated credential directory
		var inventory credentialInventory
		if err == nil && jsonkit.Unmarshal(raw, &inventory) == nil {
			pending = inventory.Entries[credentialKey(entry.Org)].Pending
		}
		if !pending {
			t.Error("refresh sent before durable pending marker")
		}
		time.Sleep(100 * time.Millisecond)
		response := protocolToken(origin)
		response.Grant.ExpiresAt = entry.GrantExpiry
		_ = jsonkit.MarshalWrite(w, response)
	}))
	defer server.Close()
	origin = server.URL
	entry = seedCredential(t, origin, true)
	close(ready)
	a, b := credentialChild(entry.Org, false), credentialChild(entry.Org, false)
	if err := a.Start(); err != nil {
		t.Fatal(err)
	}
	outputB, errB := b.CombinedOutput()
	errA := a.Wait()
	if errA != nil || errB != nil {
		t.Fatalf("children failed %v %v: %s", errA, errB, outputB)
	}
	if posts.Load() != 1 {
		t.Fatalf("refresh POSTs=%d", posts.Load())
	}
	authorize, err := (Manager{}).Authorize(context.Background(), entry.Org)
	if err != nil {
		t.Fatal(err)
	}
	assertAuthorizationToken(t, authorize, entry.Org, "new-access")
}

func TestRefreshCachesOrgNameWithoutChangingIdentity(t *testing.T) {
	for _, tc := range []struct{ cached, returned, want string }{
		{"", "", "selected"},
		{"Original", "", "Original"},
		{"Original", "Renamed ทีม", "Renamed ทีม"},
	} {
		t.Run(tc.cached+"/"+tc.returned, func(t *testing.T) {
			t.Setenv("ANFRA_HOME", t.TempDir())
			var entry storedCredential
			var origin string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				response := protocolToken(origin)
				response.Grant.Org.Name = tc.returned
				response.Grant.ExpiresAt = entry.GrantExpiry
				_ = jsonkit.MarshalWrite(w, response)
			}))
			defer server.Close()
			origin = server.URL
			entry = seedCredential(t, origin, true)
			entry.OrgName = tc.cached
			if err := withCredentialLock(context.Background(), func(inventory *credentialInventory, path string) error {
				inventory.Entries[credentialKey(entry.Org)] = entry
				return saveCredentialInventory(path, inventory)
			}); err != nil {
				t.Fatal(err)
			}
			authorize, err := (Manager{}).Authorize(context.Background(), entry.Org)
			if err != nil {
				t.Fatal(err)
			}
			assertAuthorizationToken(t, authorize, entry.Org, "new-access")
			logins, err := (Manager{}).StoredLogins(context.Background())
			if err != nil || len(logins) != 1 || logins[0].Org != entry.Org || logins[0].Name != tc.want || logins[0].LocalState != "stored" {
				t.Fatalf("%v %v", logins, err)
			}
		})
	}
}

func TestCrashDuringRefreshBlocksPredecessor(t *testing.T) {
	t.Setenv("ANFRA_HOME", t.TempDir())
	arrived := make(chan struct{}, 1)
	release := make(chan struct{})
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		arrived <- struct{}{}
		<-release
		http.Error(w, "lost response", 500)
	}))
	defer server.Close()
	defer close(release)
	entry := seedCredential(t, server.URL, true)
	child := credentialChild(entry.Org, false)
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill() }()
	select {
	case <-arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("child never sent refresh")
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()
	recovery := credentialChild(entry.Org, true)
	output, err := recovery.CombinedOutput()
	if err != nil {
		t.Fatalf("crash recovery failed: %v %s", err, output)
	}
	if posts.Load() != 1 {
		t.Fatalf("reused predecessor: %d requests", posts.Load())
	}
}

func TestUncertainRefreshAndIdentityChangesStayPending(t *testing.T) {
	for _, failure := range []string{"server error", "malformed", "grant changed", "org changed", "deadline changed", "refresh reused"} {
		t.Run(failure, func(t *testing.T) {
			t.Setenv("ANFRA_HOME", t.TempDir())
			var entry storedCredential
			var posts atomic.Int32
			var origin string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				posts.Add(1)
				response := protocolToken(origin)
				response.Grant.ExpiresAt = entry.GrantExpiry
				switch failure {
				case "server error":
					http.Error(w, "secret old-refresh", 500)
					return
				case "malformed":
					_, _ = w.Write([]byte(`{"refresh_token":"new-refresh"`))
					return
				case "grant changed":
					response.Grant.ID = "33333333-3333-4333-8333-333333333333"
				case "org changed":
					response.Grant.Org.ID = "33333333-3333-4333-8333-333333333333"
				case "deadline changed":
					response.Grant.ExpiresAt = response.Grant.ExpiresAt.Add(time.Second)
				case "refresh reused":
					response.RefreshToken = entry.RefreshToken
				}
				_ = jsonkit.MarshalWrite(w, response)
			}))
			defer server.Close()
			origin = server.URL
			entry = seedCredential(t, origin, true)
			for range 2 {
				authorize, err := (Manager{}).Authorize(context.Background(), entry.Org)
				if err == nil || authorize != nil || strings.Contains(err.Error(), "old-refresh") {
					t.Fatalf("unsafe failure %v", err)
				}
			}
			if posts.Load() != 1 {
				t.Fatalf("retried uncertain refresh: %d", posts.Load())
			}
			err := withCredentialLock(context.Background(), func(inventory *credentialInventory, _ string) error {
				if !inventory.Entries[credentialKey(entry.Org)].Pending {
					t.Error("pending was cleared")
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPendingBlocksStillValidAccessAndInventoryIsOffline(t *testing.T) {
	t.Setenv("ANFRA_HOME", t.TempDir())
	var network atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { network.Add(1); http.Error(w, "offline", 500) }))
	defer server.Close()
	entry := seedCredential(t, server.URL, false)
	if err := withCredentialLock(context.Background(), func(inventory *credentialInventory, path string) error {
		entry.Pending = true
		inventory.Entries[credentialKey(entry.Org)] = entry
		return saveCredentialInventory(path, inventory)
	}); err != nil {
		t.Fatal(err)
	}
	if authorize, err := (Manager{}).Authorize(context.Background(), entry.Org); err == nil || authorize != nil {
		t.Fatal("pending exposed valid predecessor access")
	}
	logins, err := (Manager{}).StoredLogins(context.Background())
	if err != nil || len(logins) != 1 || logins[0].LocalState != "sign_in_again" || network.Load() != 0 {
		t.Fatalf("inventory=%v err=%v network=%d", logins, err, network.Load())
	}
}

func TestLogoutRemovesSelectedEntryWhenOffline(t *testing.T) {
	t.Setenv("ANFRA_HOME", t.TempDir())
	var token string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		token = r.Form.Get("token")
		http.Error(w, "offline", 500)
	}))
	defer server.Close()
	entry := seedCredential(t, server.URL, false)
	other := entry
	other.Org.Slug = "other"
	other.RefreshToken = "other-refresh"
	if err := withCredentialLock(context.Background(), func(inventory *credentialInventory, path string) error {
		inventory.Entries[credentialKey(other.Org)] = other
		return saveCredentialInventory(path, inventory)
	}); err != nil {
		t.Fatal(err)
	}
	result, err := (Manager{}).Logout(context.Background(), entry.Org)
	if err == nil || !result.LocalRemoved || result.RemoteRevocationConfirmed || !strings.Contains(err.Error(), server.URL+"/account/connected-apps") || token != "old-refresh" {
		t.Fatalf("logout %v token=%q", err, token)
	}
	logins, err := (Manager{}).StoredLogins(context.Background())
	if err != nil || len(logins) != 1 || logins[0].Org != other.Org {
		t.Fatalf("remaining entries %v %v", logins, err)
	}
}

func TestFailedSuccessorSaveRetainsRecovery(t *testing.T) {
	t.Setenv("ANFRA_HOME", t.TempDir())
	path := os.Getenv("ANFRA_HOME") + "/credentials.json"
	var entry storedCredential
	var origin string
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		// A real filesystem obstruction, after the pending write and HTTP request,
		// prevents successor rename without adding a production failure hook.
		if err := os.Rename(path, path+".backup"); err != nil { //nolint:gosec // G703: simulate failure in this test's isolated credential directory
			t.Error(err)
		}
		if err := os.Mkdir(path, 0o700); err != nil { //nolint:gosec // G703: simulate failure in this test's isolated credential directory
			t.Error(err)
		}
		response := protocolToken(origin)
		response.Grant.ExpiresAt = entry.GrantExpiry
		_ = jsonkit.MarshalWrite(w, response)
	}))
	defer server.Close()
	origin = server.URL
	entry = seedCredential(t, origin, true)
	if authorize, err := (Manager{}).Authorize(context.Background(), entry.Org); err == nil || authorize != nil {
		t.Fatal("exposed unsaved successor")
	}
	if err := os.Remove(path); err != nil { //nolint:gosec // G703: this test's isolated credential path
		t.Fatal(err)
	}
	if err := os.Rename(path+".backup", path); err != nil { //nolint:gosec // G703: this test's isolated credential path
		t.Fatal(err)
	}
	if authorize, err := (Manager{}).Authorize(context.Background(), entry.Org); err == nil || authorize != nil {
		t.Fatal("reused predecessor after failed save")
	}
	if posts.Load() != 1 {
		t.Fatalf("retried after save failure: %d", posts.Load())
	}
}

func TestRecoveryMarkerBlocksInterruptedSuccessorCommit(t *testing.T) {
	t.Setenv("ANFRA_HOME", t.TempDir())
	entry := seedCredential(t, "https://example.com", false)
	err := withCredentialLock(context.Background(), func(inventory *credentialInventory, path string) error {
		// Model a crash between the successor's directory sync and journal clear.
		return markRefreshRecovery(path, entry.Org)
	})
	if err != nil {
		t.Fatal(err)
	}
	if authorize, err := (Manager{}).Authorize(context.Background(), entry.Org); err == nil || authorize != nil {
		t.Fatal("interrupted commit exposed access token")
	}
}

func TestCancelledLogoutStillRemovesCredentials(t *testing.T) {
	t.Setenv("ANFRA_HOME", t.TempDir())
	entry := seedCredential(t, "https://example.com", false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := (Manager{}).Logout(ctx, entry.Org)
	if err == nil || !result.LocalRemoved || result.RemoteRevocationConfirmed {
		t.Fatalf("logout=%v", err)
	}
	logins, err := (Manager{}).StoredLogins(context.Background())
	if err != nil || len(logins) != 0 {
		t.Fatalf("cancelled logout retained credentials: %v %v", logins, err)
	}
}

func TestAuthorizationReloadsAfterAnotherProcessRotates(t *testing.T) {
	t.Setenv("ANFRA_HOME", t.TempDir())
	var entry storedCredential
	var origin string
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		response := protocolToken(origin)
		response.Grant.ExpiresAt = entry.GrantExpiry
		_ = jsonkit.MarshalWrite(w, response)
	}))
	defer server.Close()
	origin = server.URL
	entry = seedCredential(t, origin, false)
	authorize, err := (Manager{}).Authorize(context.Background(), entry.Org)
	if err != nil {
		t.Fatal(err)
	}
	assertAuthorizationToken(t, authorize, entry.Org, "old-access")
	// Make the token eligible for refresh, then let another OS process rotate.
	if err := withCredentialLock(context.Background(), func(inventory *credentialInventory, path string) error {
		entry.AccessExpiry = time.Now().Add(time.Minute)
		inventory.Entries[credentialKey(entry.Org)] = entry
		return saveCredentialInventory(path, inventory)
	}); err != nil {
		t.Fatal(err)
	}
	if output, err := credentialChild(entry.Org, false).CombinedOutput(); err != nil {
		t.Fatalf("competing refresh: %v %s", err, output)
	}
	assertAuthorizationToken(t, authorize, entry.Org, "new-access")
	if posts.Load() != 1 {
		t.Fatalf("refresh requests=%d", posts.Load())
	}
}
