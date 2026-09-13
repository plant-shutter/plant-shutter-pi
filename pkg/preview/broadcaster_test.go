package preview

import (
	"bytes"
	"testing"
	"time"
)

func TestBroadcasterPublishesAndRemovesClients(t *testing.T) {
	b := New()
	ch := b.Add(nil)
	if b.ClientCount() != 1 {
		t.Fatal("client was not registered")
	}
	b.Publish([]byte{1, 2, 3})
	select {
	case got := <-ch:
		if len(got) != 3 {
			t.Fatalf("frame length %d", len(got))
		}
	case <-time.After(time.Second):
		t.Fatal("frame not published")
	}
	b.Remove(nil)
	if b.ClientCount() != 0 {
		t.Fatal("client was not removed")
	}
}

func TestSplitAnnexB(t *testing.T) {
	input := []byte{0, 0, 1, 0x67, 1, 2, 0, 0, 0, 1, 0x68, 3, 0, 0, 1, 0x65, 4}
	got := splitAnnexB(input)
	want := [][]byte{{0, 0, 0, 1, 0x67, 1, 2}, {0, 0, 0, 1, 0x68, 3}, {0, 0, 0, 1, 0x65, 4}}
	if len(got) != len(want) {
		t.Fatalf("got %d NALs, want %d", len(got), len(want))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Errorf("NAL %d = % x, want % x", i, got[i], want[i])
		}
	}
}

func TestBroadcasterReplaysHeadersAndKeyframe(t *testing.T) {
	b := New()
	b.Publish([]byte{
		0, 0, 1, 0x67, 1,
		0, 0, 1, 0x68, 2,
		0, 0, 1, 0x65, 3,
	})

	ch := b.Add(nil)
	for i, wantType := range []byte{7, 8, 5} {
		select {
		case got := <-ch:
			if nalType(got) != wantType {
				t.Fatalf("replayed NAL %d has type %d, want %d", i, nalType(got), wantType)
			}
		case <-time.After(time.Second):
			t.Fatalf("replayed NAL %d not received", i)
		}
	}
}

func TestHandlerReplacesPumpAfterModeSwitch(t *testing.T) {
	h := &Handler{Hub: New()}
	first := make(chan []byte)
	second := make(chan []byte)

	h.startPump(first)
	h.startPump(second)

	h.pumpMu.Lock()
	active := h.pumpFrames
	h.pumpMu.Unlock()
	if active != second {
		t.Fatal("mode switch did not replace the stale preview pump")
	}

	close(second)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		h.pumpMu.Lock()
		stopped := h.pumpCancel == nil
		h.pumpMu.Unlock()
		if stopped {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("replacement preview pump did not stop")
}
