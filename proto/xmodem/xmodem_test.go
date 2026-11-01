package xmodem

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"testing"
	"time"
)

// fastTiming makes the waits short for the in-process tests
func fastTiming(t *testing.T) {
	saved := timing
	timing.start = 5 * time.Second
	timing.startTry = 100 * time.Millisecond
	timing.reply = 500 * time.Millisecond
	timing.block = 300 * time.Millisecond
	timing.char = 50 * time.Millisecond
	timing.purge = 20 * time.Millisecond
	timing.poll = 10 * time.Millisecond
	t.Cleanup(func() { timing = saved })
}

// pair makes two links joined to each other. fwd changes what the sender
// (a) writes, back what the receiver (b) writes; nil passes all.
func pair(fwd, back func([]byte) []byte) (a, b *ChanLink) {
	a = NewChanLink(func(p []byte) error {
		if fwd != nil {
			p = fwd(p)
		}
		b.Feed(p)
		return nil
	})
	b = NewChanLink(func(p []byte) error {
		if back != nil {
			p = back(p)
		}
		a.Feed(p)
		return nil
	})
	return a, b
}

// testData is random bytes that don't end with 0x1A, as XModem cuts it
func testData(n int) []byte {
	r := rand.New(rand.NewPCG(uint64(n), 1))
	d := make([]byte, n)
	for i := range d {
		d[i] = byte(r.UintN(256))
	}
	if n > 0 && d[n-1] == sub {
		d[n-1] = 0
	}
	return d
}

type result struct {
	name string
	data []byte
	err  error
}

// transfer runs Send and Receive at once and returns both results
func transfer(ctx context.Context, p Protocol, name string, data []byte, fwd, back func([]byte) []byte) (sendErr error, got result) {
	a, b := pair(fwd, back)
	var wg sync.WaitGroup
	wg.Go(func() {
		got.name, got.data, got.err = Receive(ctx, b, p, nil)
	})
	sendErr = Send(ctx, a, p, name, data, nil)
	wg.Wait()
	return sendErr, got
}

func check(t *testing.T, p Protocol, data []byte, sendErr error, got result) {
	t.Helper()
	if sendErr != nil {
		t.Fatalf("send: %v", sendErr)
	}
	if got.err != nil {
		t.Fatalf("receive: %v", got.err)
	}
	if !bytes.Equal(got.data, data) {
		t.Fatalf("got %d bytes, want %d; equal prefix: %v", len(got.data), len(data),
			bytes.HasPrefix(got.data, data) || bytes.HasPrefix(data, got.data))
	}
	if p == YModem && got.name != "file.bin" {
		t.Fatalf("name %q", got.name)
	}
}

var sizes = []int{0, 1, 127, 128, 129, 1023, 1024, 5000}

func TestTransfer(t *testing.T) {
	fastTiming(t)
	for _, p := range []Protocol{XModem, XModem1K, YModem} {
		for _, n := range sizes {
			if n == 0 && p != YModem {
				continue
			}
			t.Run(fmt.Sprintf("%v/%d", p, n), func(t *testing.T) {
				data := testData(n)
				sendErr, got := transfer(t.Context(), p, "dir/file.bin", data, nil, nil)
				check(t, p, data, sendErr, got)
			})
		}
	}
}

// YModem keeps trailing 0x1A bytes, as it knows the size
func TestYModemKeepsSub(t *testing.T) {
	fastTiming(t)
	data := append(testData(300), sub, sub, sub)
	sendErr, got := transfer(t.Context(), YModem, "file.bin", data, nil, nil)
	check(t, YModem, data, sendErr, got)
}

// A receiver of XModem whose 'C' is lost falls back to the checksum
func TestChecksumFallback(t *testing.T) {
	fastTiming(t)
	dropC := func(p []byte) []byte { return bytes.ReplaceAll(p, []byte{crcReq}, nil) }
	var mu sync.Mutex
	frames := map[int]int{}
	count := func(p []byte) []byte {
		mu.Lock()
		frames[len(p)]++
		mu.Unlock()
		return p
	}
	data := testData(1000)
	sendErr, got := transfer(t.Context(), XModem, "", data, count, dropC)
	check(t, XModem, data, sendErr, got)
	if frames[132] == 0 || frames[133] != 0 {
		t.Fatalf("frames by length: %v", frames)
	}
}

// lossy corrupts or drops bytes of a fifth of the sender's writes; the first byte of
// a write (SOH, STX, EOT) is kept, as noise that makes a false EOT would
// end any XModem early
func lossy(seed uint64) func([]byte) []byte {
	var mu sync.Mutex
	r := rand.New(rand.NewPCG(seed, 2))
	return func(p []byte) []byte {
		mu.Lock()
		defer mu.Unlock()
		q := append([]byte(nil), p...)
		switch x := r.UintN(20); {
		case x == 0:
			return nil // lost whole
		case x <= 2 && len(q) > 1:
			q[1+r.IntN(len(q)-1)] ^= byte(1 + r.UintN(255)) // corrupted
		case x == 3 && len(q) > 1:
			i := 1 + r.IntN(len(q)-1)
			q = append(q[:i], q[i+1:]...) // a byte lost
		}
		return q
	}
}

// dropACKs loses some ACKs of the receiver, so the sender repeats a block
func dropACKs(which ...int) func([]byte) []byte {
	var mu sync.Mutex
	n := 0
	return func(p []byte) []byte {
		mu.Lock()
		defer mu.Unlock()
		if len(p) == 1 && p[0] == ack {
			n++
			for _, w := range which {
				if n == w {
					return nil
				}
			}
		}
		return p
	}
}

func TestLossy(t *testing.T) {
	fastTiming(t)
	for _, p := range []Protocol{XModem, XModem1K, YModem} {
		for seed := range uint64(3) {
			t.Run(fmt.Sprintf("%v/%d", p, seed), func(t *testing.T) {
				data := testData(5000)
				sendErr, got := transfer(t.Context(), p, "file.bin", data, lossy(seed), dropACKs(1, 3, 5))
				check(t, p, data, sendErr, got)
			})
		}
	}
}

// Cancelling the sender's context stops both sides
func TestCancelSender(t *testing.T) {
	fastTiming(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	a, b := pair(nil, nil)
	var got result
	var wg sync.WaitGroup
	wg.Go(func() { got.name, got.data, got.err = Receive(t.Context(), b, XModem1K, nil) })
	start := time.Now()
	err := Send(ctx, a, XModem1K, "", testData(100000), func(done, total int) {
		if done > 10000 {
			cancel()
		}
	})
	wg.Wait()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("send: %v", err)
	}
	if got.err != ErrCancelled {
		t.Fatalf("receive: %v", got.err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("took %v", d)
	}
}

// Cancelling the receiver's context stops both sides
func TestCancelReceiver(t *testing.T) {
	fastTiming(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	a, b := pair(nil, nil)
	var got result
	var wg sync.WaitGroup
	wg.Go(func() {
		got.name, got.data, got.err = Receive(ctx, b, YModem, func(done, total int) {
			if done > 3000 {
				cancel()
			}
		})
	})
	err := Send(t.Context(), a, YModem, "file.bin", testData(100000), nil)
	wg.Wait()
	if err != ErrCancelled {
		t.Fatalf("send: %v", err)
	}
	if !errors.Is(got.err, context.Canceled) {
		t.Fatalf("receive: %v", got.err)
	}
}

// A wait ends at once when the context is cancelled, even with the long
// default waits
func TestCancelWait(t *testing.T) {
	a, _ := pair(nil, nil)
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := Send(ctx, a, XModem, "", testData(10), nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("send: %v", err)
	}
	_, b := pair(nil, nil)
	ctx2, cancel2 := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel2()
	_, _, err = Receive(ctx2, b, XModem, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("receive: %v", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("took %v", d)
	}
}

// A receiver that NAKs every block makes the sender give up
func TestTooManyErrors(t *testing.T) {
	fastTiming(t)
	var a *ChanLink
	a = NewChanLink(func(p []byte) error {
		if len(p) > 1 || p[0] == eot {
			a.Feed([]byte{nak})
		}
		return nil
	})
	a.Feed([]byte{crcReq})
	if err := Send(t.Context(), a, XModem, "", testData(10), nil); err != ErrTooManyErrors {
		t.Fatalf("send: %v", err)
	}
}

func TestHeader(t *testing.T) {
	h := header(`C:\dir\name.txt`, 1234)
	if len(h) != 128 {
		t.Fatalf("len %d", len(h))
	}
	name, size := parseHeader(h)
	if name != "name.txt" || size != 1234 {
		t.Fatalf("%q %d", name, size)
	}
	if name, size := parseHeader(make([]byte, 128)); name != "" || size != -1 {
		t.Fatalf("%q %d", name, size)
	}
}
