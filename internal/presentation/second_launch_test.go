package presentation

import (
	"strings"
	"testing"
	"time"

	settingsapp "github.com/iotus3/simplelife/app/internal/application/settings"
	"github.com/iotus3/simplelife/app/internal/presentation/i18n"
)

// A later start does not open a second window: it is handed to this copy, which comes forward
// and says so. Without the sentence a window that reappears looks like one that just started.
func TestAWokenWindowSaysItWasAlreadyRunning(t *testing.T) {
	fixture := newFixture(t)
	session := fixture.openWindow(t)
	settle(t, session)

	before := fixture.state.toaster.Len()
	fixture.state.noteSecondLaunch()
	settle(t, session)

	if fixture.state.toaster.Len() <= before {
		t.Fatal("a second start must leave a message, not just a window")
	}
	if fixture.state.secondLaunch.Load() {
		t.Fatal("the flag is consumed by the paint that shows the message")
	}
}

// The channel a real claim hands launches to is what feeds that flag: one message
// per hand-off, and the last one is not lost while the window is still starting.
func TestEveryHandedOffLaunchIsCounted(t *testing.T) {
	fixture := newFixture(t)
	launches := make(chan struct{}, 1)
	shown := 0
	fixture.state.watchSecondLaunch(func() { shown++ }, launches)

	launches <- struct{}{}
	deadline := time.Now().Add(2 * time.Second)
	for !fixture.state.secondLaunch.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !fixture.state.secondLaunch.Load() {
		t.Fatal("a handed-off launch did not reach the window")
	}

	// The buffer holds one: two clicks during startup are one request to come
	// forward, which is what the claim writes.
	launches <- struct{}{}
	launches <- struct{}{}
	if len(launches) != 1 {
		t.Fatalf("launches buffered = %d, want 1 (coalesced)", len(launches))
	}
}

// Hiding the window to the status area has to say where it went — once in the life
// of the installation, not once per launch: someone who closes the window every
// morning should be told once, not every morning.
func TestTheTrayNotificationIsSentOnceEver(t *testing.T) {
	fixture := newFixture(t)
	var sent []string
	fixture.state.deps.Notify = func(title, body string) {
		sent = append(sent, title+" | "+body)
	}

	fixture.state.noteHiddenToTray()
	fixture.state.noteHiddenToTray()

	if len(sent) != 1 {
		t.Fatalf("notifications = %d, want 1", len(sent))
	}
	want := i18n.T(fixture.state.lang, i18n.MsgHiddenToTray)
	if body := sent[0]; !strings.Contains(body, want) {
		t.Fatalf("notification %q does not name the status area (%q)", body, want)
	}
	// The promise lives in the settings, so it survives the next launch.
	if !fixture.deps.Settings.TrayHintShown() {
		t.Fatal("the notice was not recorded as shown")
	}
}

// A later launch reads the stored flag: the sentence was said once, and a new
// process does not repeat it.
func TestANewLaunchDoesNotRepeatTheTrayNotification(t *testing.T) {
	fixture := newFixture(t)
	fixture.state.noteHiddenToTray()

	// A second start over the same database: the settings service is the only thing
	// that carries over, which is exactly what the flag has to live in.
	fresh := settingsapp.New(fixture.store.Settings())
	if !fresh.TrayHintShown() {
		t.Fatal("the flag did not come back from the store")
	}
}

// A build without a notifier (a test, a platform without one) simply says nothing:
// hiding the window must never fail because the desktop cannot show a notification,
// and the sentence counts as said so it is not attempted again.
func TestHidingWithoutANotifierIsQuiet(t *testing.T) {
	fixture := newFixture(t)
	fixture.state.deps.Notify = nil
	fixture.state.noteHiddenToTray()
	if !fixture.deps.Settings.TrayHintShown() {
		t.Fatal("the notice is marked as used even when nobody could be told")
	}
}
