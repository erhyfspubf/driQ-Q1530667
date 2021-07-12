package spec

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/qri-io/qri/automation/trigger"
	"github.com/qri-io/qri/automation/workflow"
	"github.com/qri-io/qri/event"
)

// AssertTrigger confirms the expected behavior of a trigger.Trigger Interface
// implementation
func AssertTrigger(t *testing.T, trig trigger.Trigger) {
	if trig.Type() == "" {
		t.Error("Type method must return a non-empty trigger.Type")
	}
	if err := trig.SetActive(true); err != nil {
		t.Fatalf("trigger.SetActive unexpected error: %s", err)
	}
	if !trig.Active() {
		t.Error("expected trigger.Active() to be true after trigger.SetActive(true)")
	}
	if err := trig.SetActive(false); err != nil {
		t.Fatalf("trigger.SetActive unexpected error: %s", err)
	}
	if trig.Active() {
		t.Error("expected trigger.Active() to be false after trigger.SetActive(false)")
	}
	triggerBytes, err := json.Marshal(trig)
	if err != nil {
		t.Fatalf("json.Marshal unexpected error: %s", err)
	}
	triggerObj := map[string]interface{}{}
	if err := json.Unmarshal(triggerBytes, &triggerObj); err != nil {
		t.Fatalf("json.Unmarshal unexpected error: %s", err)
	}
	triggerType, ok := triggerObj["type"]
	if !ok {
		t.Fatal("json.Marshal error, expected Type field to exist")
	}
	if triggerType != trig.Type() {
		t.Fatalf("json.Marshal error, expected marshalled type %q to match trigger.Type() %q", triggerType, trig.Type())
	}
	if err := json.Unmarshal(triggerBytes, &triggerObj); err != nil {
		t.Fatalf("json.Unmarshal unexpected error: %s", err)
	}

	triggerObj["type"] = "bad trigger type"
	triggerBytes, err = json.Marshal(triggerObj)
	if err != nil {
		t.Fatalf("json.Marshal unexpected error: %s", err)
	}
	if err := json.Unmarshal(triggerBytes, trig); !errors.Is(err, trigger.ErrUnexpectedType) {
		t.Fatalf("json.Unmarshal should emit a `trigger.ErrUnexpectedType` error if the given type does not match the trigger.Type of the Trigger")
	}
}

// ListenerConstructor creates a trigger listener and function that fires the listener when called
type ListenerConstructor func(ctx context.Context, bus event.Bus) (listener trigger.Listener, activate func())

// AssertListener confirms the expected behavior of a trigger.Listener
// NOTE: this does not confirm behavior of the `Listen` functionality
// beyond the basic usage of adding a trigger using a `trigger.Source`
func AssertListener(t *testing.T, listenerConstructor ListenerConstructor) {
	ctx := context.Background()
	bus := event.NewBus(ctx)
	listener, activateTrigger := listenerConstructor(ctx, bus)
	wf := &workflow.Workflow{}
	if err := listener.Listen(wf); !errors.Is(err, trigger.ErrEmptyWorkflowID) {
		t.Fatal("listener.Listen should emit a trigger.ErrEmptyWorkflowID if the WorkflowID of the trigger.Source is empty")
	}
	wf = &workflow.Workflow{ID: "workflow_id"}
	if err := listener.Listen(wf); !errors.Is(err, trigger.ErrEmptyOwnerID) {
		t.Fatal("listener.Listen should emit a trigger.ErrEmptyOwnerID if the OwnerID if the trigger.Source is emtpy")
	}

	triggered := make(chan string)
	handler := func(ctx context.Context, e event.Event) error {
		if e.Type == event.ETWorkflowTrigger {
			triggered <- "triggered!"
		}
		return nil
	}
	bus.SubscribeTypes(handler, event.ETWorkflowTrigger)
	done := shouldTimeout(t, triggered, "listener should not emit events until the listener has been started by running `listener.Start()`")
	activateTrigger()
	<-done

	if err := listener.Start(ctx); err != nil {
		t.Fatalf("listener.Start unexpected error: %s", err)
	}
	done = errOnTimeout(t, triggered, "listener did not emit an event.ETWorkflowTrigger event when the trigger was activated")
	activateTrigger()
	<-done

	if err := listener.Stop(); err != nil {
		t.Fatalf("listener.Stop unexpected error: %s", err)
	}
	done = shouldTimeout(t, triggered, "listener should not emit events once the listener has run `listener.Stop()`")
	activateTrigger()
	<-done
}

func errOnTimeout(t *testing.T, c chan string, errMsg string) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		select {
		case msg := <-c:
			t.Log(msg)
		case <-time.After(200 * time.Millisecond):
			t.Errorf(errMsg)
		}
		done <- struct{}{}
	}()
	return done
}

func shouldTimeout(t *testing.T, c chan string, errMsg string) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		select {
		case badMsg := <-c:
			t.Errorf(badMsg)
		case <-time.After(200 * time.Millisecond):
			t.Log("expected timeout")
		}
		done <- struct{}{}
	}()
	return done
}