//go:build windows

package core

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func testCodexNotifyIconRepair(
	find func(codexNotifyIconTarget) uintptr,
	post func(uintptr) error,
) codexNotifyIconRepair {
	return codexNotifyIconRepair{
		find:   find,
		post:   post,
		appear: 500 * time.Millisecond,
		poll:   time.Millisecond,
		settle: time.Millisecond,
		retry:  time.Millisecond,
	}
}

func TestCodexNotifyIconTargetMatchesOnlyTheExpectedProcess(t *testing.T) {
	target := codexNotifyIconTarget{processID: 1234}
	if !target.matches(1234, `C:\somewhere\ChatGPT.exe`) {
		t.Fatal("the expected process was rejected")
	}
	if target.matches(4321, `C:\somewhere\ChatGPT.exe`) {
		t.Fatal("another ChatGPT.exe process was accepted")
	}
	if target.matches(0, `C:\somewhere\ChatGPT.exe`) {
		t.Fatal("a window without an owning process was accepted")
	}
}

func TestCodexNotifyIconTargetMatchesTheFullExecutablePath(t *testing.T) {
	target := codexNotifyIconTarget{executablePath: `C:\Programs\Codex\ChatGPT.exe`}
	if !target.matches(1234, `C:\Programs\Codex\ChatGPT.exe`) {
		t.Fatal("the launched executable was rejected")
	}
	if !target.matches(1234, `C:/Programs/Codex/ChatGPT.exe`) {
		t.Fatal("the launched executable was rejected because of the separators")
	}
	if target.matches(1234, `C:\Other\ChatGPT.exe`) {
		t.Fatal("a ChatGPT.exe from another directory was accepted")
	}
	if target.matches(1234, "") {
		t.Fatal("a process without a readable image path was accepted")
	}
}

func TestCodexNotifyIconRepairNeedsAnIdentifiedInstance(t *testing.T) {
	repair := testCodexNotifyIconRepair(
		func(codexNotifyIconTarget) uintptr { t.Fatal("an unidentified instance was looked up"); return 0 },
		func(uintptr) error { t.Fatal("an unidentified instance was posted to"); return nil },
	)
	if err := repair.run(context.Background(), codexNotifyIconTarget{}); err == nil {
		t.Fatal("an unidentified instance was accepted")
	}
}

func TestCodexNotifyIconRepairLooksUpTheWindowAgainAfterTheSettleDelay(t *testing.T) {
	lookups := 0
	posted := uintptr(0)
	repair := testCodexNotifyIconRepair(
		func(codexNotifyIconTarget) uintptr {
			lookups++
			if lookups == 1 {
				return 0x11
			}
			return 0x22
		},
		func(window uintptr) error {
			posted = window
			return nil
		},
	)
	if err := repair.run(context.Background(), codexNotifyIconTarget{processID: 7}); err != nil {
		t.Fatal(err)
	}
	if lookups < 2 {
		t.Fatalf("the repair reused the window handle it cached before waiting: %d lookups", lookups)
	}
	if posted != 0x22 {
		t.Fatalf("posted to window %#x, want 0x22 from the lookup after the settle delay", posted)
	}
}

func TestCodexNotifyIconRepairRetriesOnceWhenPostingFails(t *testing.T) {
	posts := 0
	repair := testCodexNotifyIconRepair(
		func(codexNotifyIconTarget) uintptr { return 0x33 },
		func(uintptr) error {
			posts++
			if posts == 1 {
				return errors.New("the window went away")
			}
			return nil
		},
	)
	if err := repair.run(context.Background(), codexNotifyIconTarget{processID: 7}); err != nil {
		t.Fatal(err)
	}
	if posts != 2 {
		t.Fatalf("posted %d times, want one retry after the first failure", posts)
	}
}

func TestCodexNotifyIconRepairReportsAPostFailure(t *testing.T) {
	posts := 0
	repair := testCodexNotifyIconRepair(
		func(codexNotifyIconTarget) uintptr { return 0x44 },
		func(uintptr) error {
			posts++
			return errors.New("PostMessage failed")
		},
	)
	err := repair.run(context.Background(), codexNotifyIconTarget{processID: 7})
	if err == nil {
		t.Fatal("a failing PostMessage was reported as a successful repair")
	}
	if !strings.Contains(err.Error(), "PostMessage failed") {
		t.Fatalf("the reported error lost the reason: %v", err)
	}
	if posts != 2 {
		t.Fatalf("posted %d times, want the attempt plus one retry", posts)
	}
}

func TestCodexNotifyIconRepairStopsWhenTheContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repair := testCodexNotifyIconRepair(
		func(codexNotifyIconTarget) uintptr { return 0x55 },
		func(uintptr) error {
			t.Fatal("the repair posted after its context was cancelled")
			return nil
		},
	)
	if err := repair.run(ctx, codexNotifyIconTarget{processID: 7}); !errors.Is(err, context.Canceled) {
		t.Fatalf("run() = %v, want a cancelled context", err)
	}
}

func TestCodexNotifyIconRepairFailsWhenTheHostWindowNeverAppears(t *testing.T) {
	repair := testCodexNotifyIconRepair(
		func(codexNotifyIconTarget) uintptr { return 0 },
		func(uintptr) error {
			t.Fatal("the repair posted without a window")
			return nil
		},
	)
	target := codexNotifyIconTarget{processID: 4242}
	started := time.Now()
	err := repair.run(context.Background(), target)
	if err == nil {
		t.Fatal("a missing icon host window was reported as a successful repair")
	}
	if !strings.Contains(err.Error(), "pid 4242") {
		t.Fatalf("the reported error does not name the instance: %v", err)
	}
	if waited := time.Since(started); waited > 5*time.Second {
		t.Fatalf("the repair waited %s, want it to give up after its own timeout", waited)
	}
}

