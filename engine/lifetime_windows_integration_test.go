//go:build windows && integration

package engine

// These start a real, headed Chrome (parked offscreen), so they sit behind the
// integration tag:  go test -tags integration -run Lifetime ./engine/

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func chromePath(t *testing.T) string {
	t.Helper()
	p := os.Getenv("AMTUI_CHROME")
	if p == "" {
		p = `C:\Program Files\Google\Chrome\Application\chrome.exe`
	}
	if _, err := os.Stat(p); err != nil {
		t.Skipf("Chrome unavailable at %q: %v", p, err)
	}
	return p
}

func waitOwner(dir string, want bool) bool {
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); {
		if (profileOwner(dir) != 0) == want {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func chromeArgs(dir string) []string {
	return []string{"--user-data-dir=" + dir, "--no-first-run", "--no-default-browser-check",
		"--window-position=-32000,-32000", "--window-size=1000,700", "about:blank"}
}

func TestLifetimeClearsBrowserOrphanedByDeadParent(t *testing.T) {
	chrome := chromePath(t)
	dir := t.TempDir()
	// cmd's start detaches Chrome and cmd exits: the browser's parent is gone,
	// exactly what an amtui killed mid-load leaves behind.
	args := append([]string{"/c", "start", "", chrome}, chromeArgs(dir)...)
	if err := exec.Command("cmd", args...).Run(); err != nil {
		t.Fatal(err)
	}
	if !waitOwner(dir, true) {
		t.Fatal("orphan browser never registered its profile window")
	}
	if err := clearStaleBrowser(dir); err != nil {
		t.Fatalf("clearStaleBrowser: %v", err)
	}
	if profileOwner(dir) != 0 {
		t.Fatal("orphan still owns the profile")
	}
}

func TestLifetimeRefusesToKillLiveAmtuisBrowser(t *testing.T) {
	chrome := chromePath(t)
	dir := t.TempDir()
	// Started by this test binary, which is alive, so it is a live owner. The
	// check matches the parent's exe name against our own, as amtui.exe would.
	cmd := exec.Command(chrome, chromeArgs(dir)...)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); waitOwner(dir, false) })
	if !waitOwner(dir, true) {
		t.Fatal("browser never registered its profile window")
	}
	if err := clearStaleBrowser(dir); err == nil {
		t.Fatal("clearStaleBrowser killed a browser whose amtui is still running")
	}
	if profileOwner(dir) == 0 {
		t.Fatal("live browser was stopped")
	}
}

// TestLifetimeHelper is the fake amtui for the test below: it launches a
// browser the way open() does, ties it, reports ready and waits to be killed.
func TestLifetimeHelper(t *testing.T) {
	dir := os.Getenv("AMTUI_LIFETIME_HELPER_DIR")
	if dir == "" {
		t.Skip("helper process only")
	}
	ctx, cancels := launch(dir, false)
	defer closeAll(cancels)
	if err := chromedp.Run(ctx); err != nil {
		t.Fatal(err)
	}
	tieBrowserToProcess(browserPID(ctx))
	os.Stdout.WriteString("ready " + strconv.Itoa(browserPID(ctx)) + "\n")
	time.Sleep(time.Minute)
}

func TestLifetimeBrowserDiesWithHardKilledAmtui(t *testing.T) {
	chromePath(t)
	dir := t.TempDir()
	self, _ := os.Executable()
	helper := exec.Command(self, "-test.run", "^TestLifetimeHelper$", "-test.v")
	helper.Env = append(os.Environ(), "AMTUI_LIFETIME_HELPER_DIR="+dir)
	out, _ := helper.StdoutPipe()
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4096)
	var got string
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); {
		n, err := out.Read(buf)
		got += string(buf[:n])
		if err != nil || len(got) > 0 && strings.Contains(got, "ready ") {
			break
		}
	}
	if !strings.Contains(got, "ready ") || profileOwner(dir) == 0 {
		helper.Process.Kill()
		t.Fatalf("helper never launched a browser: %q", got)
	}
	// TerminateProcess: no deferred cleanup, no context cancel. Only the job
	// object can take the browser down now.
	if err := helper.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	helper.Wait()
	if !waitOwner(dir, false) {
		t.Fatal("browser outlived the hard-killed amtui")
	}
}
