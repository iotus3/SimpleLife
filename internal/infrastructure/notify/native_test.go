package notify

import (
	"errors"
	"testing"

	"github.com/vivaui/viva/app"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

func TestNotifierTranslatesCodesBeforeShowingThem(t *testing.T) {
	var shown []app.Notification
	translations := map[string]string{
		"focus.done.title": "专注结束",
		"focus.done.body":  "休息一下",
	}
	n := Notifier{
		Translate: func(code string) string {
			if sentence, ok := translations[code]; ok {
				return sentence
			}
			return code
		},
		send: func(n app.Notification) error {
			shown = append(shown, n)
			return nil
		},
	}
	err := n.Notify(shared.Notification{Title: "focus.done.title", Body: "focus.done.body"})
	if err != nil {
		t.Fatalf("notify: %v", err)
	}
	if len(shown) != 1 {
		t.Fatalf("got %d notifications, want 1", len(shown))
	}
	if shown[0].Title != "专注结束" || shown[0].Body != "休息一下" {
		t.Errorf("shown %q / %q, want the translated sentences", shown[0].Title, shown[0].Body)
	}
	if n.Capability() != shared.NotifyAvailable {
		t.Errorf("capability is %v, want available", n.Capability())
	}
}

func TestNotifierShowsAnUnknownCodeAsItIs(t *testing.T) {
	var shown []app.Notification
	n := Notifier{
		Translate: func(code string) string { return code },
		send: func(n app.Notification) error {
			shown = append(shown, n)
			return nil
		},
	}
	if err := n.Notify(shared.Notification{Title: "focus.unknown", Body: "focus.unknown"}); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if len(shown) != 1 || shown[0].Title != "focus.unknown" {
		t.Fatalf("shown %+v, want the code unchanged", shown)
	}
}

func TestNotifierWithoutATranslationShowsTheCode(t *testing.T) {
	var shown []app.Notification
	n := Notifier{send: func(n app.Notification) error {
		shown = append(shown, n)
		return nil
	}}
	if err := n.Notify(shared.Notification{Title: "focus.done.title"}); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if len(shown) != 1 || shown[0].Title != "focus.done.title" {
		t.Fatalf("shown %+v, want the code unchanged", shown)
	}
}

func TestNotifierReportsWhatThePlatformSays(t *testing.T) {
	want := errors.New("no notification centre")
	n := Notifier{send: func(app.Notification) error { return want }}
	if err := n.Notify(shared.Notification{Title: "t", Body: "b"}); !errors.Is(err, want) {
		t.Fatalf("err is %v, want %v", err, want)
	}
}
