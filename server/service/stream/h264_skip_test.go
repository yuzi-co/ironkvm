package stream

import (
	"encoding/binary"
	"sync"
	"sync/atomic"
	"testing"

	"NanoKVM-Server/common"
)

// numberedEncoder stands in for the encoder: every read is the next frame of
// one stream, numbered in its data. The first is a keyframe, and after that
// only a request makes one, as libkvm-v4l2 does by default.
type numberedEncoder struct {
	mutex   sync.Mutex
	next    uint64
	keyNext bool
}

func withNumberedEncoder(t *testing.T) *numberedEncoder {
	t.Helper()

	encoder := &numberedEncoder{keyNext: true}

	originalRead, originalKind, originalRequest := readVideo, keyframeRequests, requestKeyframe
	t.Cleanup(func() {
		readVideo, keyframeRequests, requestKeyframe = originalRead, originalKind, originalRequest
	})

	readVideo = func(uint16, uint16, uint8, uint16, uint8, uint8) ([]byte, int) {
		encoder.mutex.Lock()
		defer encoder.mutex.Unlock()

		encoder.next++
		data := binary.BigEndian.AppendUint64(nil, encoder.next)
		if encoder.keyNext {
			encoder.keyNext = false

			return data, 3
		}

		return data, 1
	}
	keyframeRequests = func() common.KeyframeRequestKind { return common.KeyframeRequestV4L2 }
	requestKeyframe = func(uint8) bool {
		encoder.mutex.Lock()
		defer encoder.mutex.Unlock()

		encoder.keyNext = true

		return true
	}

	return encoder
}

// frames is how many frames the encoder has made.
func (e *numberedEncoder) frames() uint64 {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	return e.next
}

type numberedFrame struct {
	number uint64
	key    bool
}

// received takes every frame a subscription is given, as a delivery path's
// goroutine does.
type received struct {
	mutex  sync.Mutex
	frames []numberedFrame
}

func receive(subscription *H264Subscription) *received {
	r := &received{}

	go func() {
		for frame := range subscription.Frames() {
			if len(frame.Data) != 8 {
				continue
			}

			r.mutex.Lock()
			r.frames = append(r.frames, numberedFrame{binary.BigEndian.Uint64(frame.Data), frame.KeyFrame})
			r.mutex.Unlock()
		}
	}()

	return r
}

func (r *received) list() []numberedFrame {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return append([]numberedFrame(nil), r.frames...)
}

func (r *received) last() numberedFrame {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if len(r.frames) == 0 {
		return numberedFrame{}
	}

	return r.frames[len(r.frames)-1]
}

// ironkvm-dist#191. A path without demand is not read for, but another path's
// demand still moves the encoder on: an H.264 Direct viewer whose flow window
// is full beside a WebRTC viewer. The frames read meanwhile are a gap for the
// quiet path, so when it asks again it must get nothing before a keyframe, and
// the encoder must be asked for that keyframe, which by default it makes only
// on request.
func TestAPathSkippedForAFrameGetsNothingBeforeAKeyframe(t *testing.T) {
	t.Setenv(keyframeIntervalEnv, "")
	withScreenFPS(t, 30)
	encoder := withNumberedEncoder(t)

	source := newTestSource(t)

	live := source.subscribe(nil)
	defer live.Close()
	liveFrames := receive(live)

	var wanted atomic.Bool
	wanted.Store(true)
	stalled := source.subscribe(func() bool { return wanted.Load() })
	defer stalled.Close()
	stalledFrames := receive(stalled)

	waitFor(t, "the stalled path's first frames", func() bool { return len(stalledFrames.list()) >= 3 })

	// The window fills: the path stops asking, and the other one keeps the
	// encoder running.
	wanted.Store(false)
	before := liveFrames.last().number
	waitFor(t, "the encoder to move on without the stalled path", func() bool {
		return liveFrames.last().number >= before+5
	})

	resumed := encoder.frames()
	wanted.Store(true)
	waitFor(t, "the stalled path's next frame", func() bool { return stalledFrames.last().number > resumed })
	waitFor(t, "the frames after it", func() bool { return stalledFrames.last().number > resumed+3 })

	frames := stalledFrames.list()
	gap := false
	for i, frame := range frames {
		if frame.key {
			gap = gap || (i > 0 && frame.number > frames[i-1].number+1)
			continue
		}
		if i == 0 {
			t.Fatalf("the path's first frame %d is not a keyframe", frame.number)
		}
		if frame.number != frames[i-1].number+1 {
			t.Fatalf("delta frame %d was delivered after frame %d: its reference never reached this path",
				frame.number, frames[i-1].number)
		}
	}
	if !gap {
		t.Fatalf("the stalled path missed no frame, so nothing was tested: %v", frames)
	}
}

// A path that stays quiet for many frames needs one keyframe, when it can take
// it. Asking while it is quiet would make keyframes it cannot take, and asking
// per missed frame would ask on every tick.
func TestAQuietPathAsksForOneKeyframeWhenItResumes(t *testing.T) {
	pending := func() string {
		defaultKeyframePolicy.mutex.Lock()
		defer defaultKeyframePolicy.mutex.Unlock()

		return defaultKeyframePolicy.pending
	}
	defaultKeyframePolicy.keySeen()
	t.Cleanup(defaultKeyframePolicy.keySeen)

	subscription := &H264Subscription{slot: NewFrameSlotDepth[H264Frame](h264SubscriptionDepth)}

	subscription.resume()
	if got := pending(); got != "" {
		t.Fatalf("a path that missed nothing asked for a keyframe (%q)", got)
	}

	for i := 0; i < 5; i++ {
		subscription.miss()
	}
	if got := pending(); got != "" {
		t.Fatalf("a keyframe was asked for (%q) while the path could not take one", got)
	}

	subscription.resume()
	if got := pending(); got != KeyframeReasonWaiting {
		t.Fatalf("pending request = %q, want %q", got, KeyframeReasonWaiting)
	}

	// The loop has taken the request and the keyframe has not come yet. The
	// path asks for frames again on the next tick, which is the same wait
	// and not a second request.
	defaultKeyframePolicy.keySeen()
	subscription.resume()
	if got := pending(); got != "" {
		t.Fatalf("the path asked again (%q) for the same gap", got)
	}

	// The delta frames before the keyframe are held back, and it ends the wait.
	subscription.deliver(dataFrame(false))
	if take(subscription) {
		t.Fatal("a delta frame after the gap was delivered before a keyframe")
	}
	subscription.deliver(dataFrame(true))
	if !take(subscription) {
		t.Fatal("the keyframe was not delivered")
	}
	subscription.deliver(dataFrame(false))
	if !take(subscription) {
		t.Fatal("the stream did not resume after the keyframe")
	}
}
