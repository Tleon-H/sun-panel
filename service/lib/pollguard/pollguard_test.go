package pollguard

import "testing"

func TestStopInvalidatesTheInFlightCollection(t *testing.T) {
	gate := New()
	gate.Start()
	ok, current := gate.TryBegin()
	if !ok {
		t.Fatal("expected the first collection to start")
	}
	gate.Stop()
	if current() {
		t.Fatal("a collection that finishes after stop must not be applied")
	}
}

func TestSecondCollectionWaitsUntilTheFirstEnds(t *testing.T) {
	gate := New()
	gate.Start()
	ok, _ := gate.TryBegin()
	if !ok {
		t.Fatal("expected the first collection to start")
	}
	if again, _ := gate.TryBegin(); again {
		t.Fatal("a second collection started while the first was still running")
	}
	gate.End()
	if again, current := gate.TryBegin(); !again || !current() {
		t.Fatal("expected a new collection after the first ended")
	}
}

func TestStoppedGateDoesNotStart(t *testing.T) {
	gate := New()
	if ok, _ := gate.TryBegin(); ok {
		t.Fatal("a gate that has not been started must not poll")
	}
}
