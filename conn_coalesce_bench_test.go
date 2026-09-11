//go:build unit
// +build unit

/*
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package gocql

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// delayedWriter simulates network RTT by sleeping in Write. A real loopback
// conn's own scheduling overhead would swamp sub-ms delays.
type delayedWriter struct {
	delay time.Duration
}

func (d *delayedWriter) Write(p []byte) (int, error) {
	if d.delay > 0 {
		time.Sleep(d.delay)
	}
	return len(p), nil
}

func (d *delayedWriter) SetWriteDeadline(time.Time) error {
	return nil
}

// coalesceBenchOpts configures runCoalesceBench. opsPerGoroutine<=0 derives
// the count from b.N; perCallerInterval>0 paces each goroutine;
// reportPercentiles reports avg/p99 instead of us/op.
type coalesceBenchOpts struct {
	window            time.Duration
	flushThreshold    int
	concurrency       int
	payload           []byte
	latency           time.Duration
	opsPerGoroutine   int
	perCallerInterval time.Duration
	reportPercentiles bool
}

// runCoalesceBench drives opts.concurrency goroutines doing writeContext calls.
func runCoalesceBench(b *testing.B, opts coalesceBenchOpts) {
	quit := make(chan struct{})
	defer close(quit)

	w := newWriteCoalescer(&delayedWriter{delay: opts.latency}, 0, opts.window, opts.flushThreshold, quit)

	var flushes, framesFlushed int64
	w.testFlushSizeHook = func(batchSize int) {
		atomic.AddInt64(&flushes, 1)
		atomic.AddInt64(&framesFlushed, int64(batchSize))
	}

	opsPerGoroutine := opts.opsPerGoroutine
	if opsPerGoroutine <= 0 {
		opsPerGoroutine = b.N / opts.concurrency
		if opsPerGoroutine < 1 {
			opsPerGoroutine = 1
		}
	}

	var latenciesByGoroutine [][]time.Duration
	if opts.reportPercentiles {
		latenciesByGoroutine = make([][]time.Duration, opts.concurrency)
	}

	b.ResetTimer()
	start := time.Now()

	var wg sync.WaitGroup
	for g := 0; g < opts.concurrency; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			var own []time.Duration
			if opts.reportPercentiles {
				own = make([]time.Duration, 0, opsPerGoroutine)
			}
			next := time.Now()
			for i := 0; i < opsPerGoroutine; i++ {
				if opts.perCallerInterval > 0 {
					if d := time.Until(next); d > 0 {
						time.Sleep(d)
					}
					next = next.Add(opts.perCallerInterval)
					// Skip missed slots: otherwise one slow op never lets the
					// pacer catch up, and every write after it fires immediately.
					if now := time.Now(); next.Before(now) {
						next = now
					}
				}
				opStart := time.Now()
				if _, err := w.writeContext(context.Background(), opts.payload); err != nil {
					b.Error(err)
					return
				}
				if opts.reportPercentiles {
					own = append(own, time.Since(opStart))
				}
			}
			if opts.reportPercentiles {
				latenciesByGoroutine[g] = own
			}
		}(g)
	}
	wg.Wait()
	b.StopTimer()

	elapsed := time.Since(start)
	total := opsPerGoroutine * opts.concurrency
	b.ReportMetric(float64(total)/elapsed.Seconds(), "ops/sec")
	if opts.reportPercentiles {
		var opLatencies []time.Duration
		for _, own := range latenciesByGoroutine {
			opLatencies = append(opLatencies, own...)
		}
		sort.Slice(opLatencies, func(i, j int) bool { return opLatencies[i] < opLatencies[j] })
		var sum time.Duration
		for _, d := range opLatencies {
			sum += d
		}
		avg := sum / time.Duration(len(opLatencies))
		p99 := opLatencies[int(0.99*float64(len(opLatencies)-1))]
		b.ReportMetric(float64(avg.Microseconds()), "avg-us/op")
		b.ReportMetric(float64(p99.Microseconds()), "p99-us/op")
	} else {
		b.ReportMetric(1e6*elapsed.Seconds()/float64(total), "us/op")
	}
	if f := atomic.LoadInt64(&flushes); f > 0 {
		b.ReportMetric(float64(atomic.LoadInt64(&framesFlushed))/float64(f), "frames/flush")
	}
}

// BenchmarkCoalescePolicy compares NoCoalesce (window=0), FixedWait (today's
// always-200us), and RTTScaledWait (this design's coalesceWindow) across
// local/cross-AZ/cross-DC. "local" uses rtt=200us (not 0, which means
// "unmeasured" to coalesceWindow) but no simulated delay, isolating raw
// overhead; the others use the same value for both.
//
// Caveat on "local": with no write cost, nothing separates consecutive writes,
// so once a window arms the ~1ms Go timer floor keeps it armed (an op then
// costs more than the pacing interval). Real connections are separated by the
// response; see docs/coalescing-benchmark-results.md on the timer floor.
func BenchmarkCoalescePolicy(b *testing.B) {
	const defaultWaitTime = 200 * time.Microsecond
	payload := makeCoalesceBenchFrame(300)

	scenarios := []struct {
		name     string
		simDelay time.Duration // delayedWriter's simulated Write() cost
		rtt      time.Duration // input to coalesceWindow
	}{
		{"local", 0, 200 * time.Microsecond},
		{"cross-AZ", time.Millisecond, time.Millisecond},
		{"cross-DC", 20 * time.Millisecond, 20 * time.Millisecond},
	}

	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			policies := []struct {
				name   string
				window time.Duration
			}{
				{"NoCoalesce", 0},
				{"FixedWait", defaultWaitTime},
				{"RTTScaledWait", coalesceWindow(sc.rtt, defaultWaitTime)},
			}
			for _, p := range policies {
				b.Run(p.name, func(b *testing.B) {
					for _, concurrency := range []int{8, 32, 128} {
						b.Run(fmt.Sprintf("conc=%d", concurrency), func(b *testing.B) {
							runCoalesceBench(b, coalesceBenchOpts{
								window:         p.window,
								flushThreshold: coalesceFlushThreshold,
								concurrency:    concurrency,
								payload:        payload,
								latency:        sc.simDelay,
							})
						})
					}
				})
			}
		})
	}
}

// BenchmarkCoalesceFlushThreshold isolates the byte threshold's effect at
// low concurrency/near-zero latency (docs/coalescing-redesign.md F3/§2).
func BenchmarkCoalesceFlushThreshold(b *testing.B) {
	const defaultWaitTime = 200 * time.Microsecond
	payload := makeCoalesceBenchFrame(300)

	thresholds := []struct {
		name      string
		threshold int
	}{
		{"NoThreshold", 0},
		{"DefaultThreshold", coalesceFlushThreshold},
	}
	for _, th := range thresholds {
		b.Run(th.name, func(b *testing.B) {
			for _, concurrency := range []int{8, 32, 128} {
				b.Run(fmt.Sprintf("conc=%d", concurrency), func(b *testing.B) {
					runCoalesceBench(b, coalesceBenchOpts{
						window:         defaultWaitTime,
						flushThreshold: th.threshold,
						concurrency:    concurrency,
						payload:        payload,
					})
				})
			}
		})
	}
}

// BenchmarkCoalescePolicyPerShardRate targets perConnRate (docs/coalescing-
// redesign.md F4: one connection per shard, not the whole cluster's rate)
// instead of saturating, using Little's law for concurrency, and reports
// per-write avg/p99 latency alongside throughput.
func BenchmarkCoalescePolicyPerShardRate(b *testing.B) {
	const (
		defaultWaitTime = 200 * time.Microsecond
		perConnRate     = 2000 // writes/sec on this one connection, per F4
	)
	payload := makeCoalesceBenchFrame(300)

	scenarios := []struct {
		name     string
		simDelay time.Duration
		rtt      time.Duration
	}{
		{"local", 0, 200 * time.Microsecond},
		{"cross-AZ", time.Millisecond, time.Millisecond},
		{"cross-DC", 20 * time.Millisecond, 20 * time.Millisecond},
	}

	for _, sc := range scenarios {
		// Little's law: enough concurrent callers to sustain perConnRate.
		concurrency := int(float64(perConnRate)*sc.simDelay.Seconds()) + 1

		// At concurrency 1, pace explicitly to perConnRate: a free loop has
		// no think-time and would blast far faster.
		var perCallerInterval time.Duration
		if concurrency == 1 {
			perCallerInterval = time.Second / perConnRate
		}

		b.Run(sc.name, func(b *testing.B) {
			policies := []struct {
				name   string
				window time.Duration
			}{
				{"NoCoalesce", 0},
				{"FixedWait", defaultWaitTime},
				{"RTTScaledWait", coalesceWindow(sc.rtt, defaultWaitTime)},
			}
			for _, p := range policies {
				b.Run(p.name, func(b *testing.B) {
					runCoalesceBench(b, coalesceBenchOpts{
						window:            p.window,
						flushThreshold:    coalesceFlushThreshold,
						concurrency:       concurrency,
						payload:           payload,
						latency:           sc.simDelay,
						perCallerInterval: perCallerInterval,
						reportPercentiles: true,
					})
				})
			}
		})
	}
}

// BenchmarkCoalesceAllocs reports per-write B/op and allocs/op for the direct
// writer versus the coalescer, b.N-driven so -benchmem is per write.
func BenchmarkCoalesceAllocs(b *testing.B) {
	payload := makeCoalesceBenchFrame(120)
	for _, tc := range []struct {
		name   string
		window time.Duration
	}{
		{"direct", -1},
		{"coalesced/w=0", 0},
		{"coalesced/w=200us", 200 * time.Microsecond},
	} {
		b.Run(tc.name, func(b *testing.B) {
			quit := make(chan struct{})
			defer close(quit)
			var w contextWriter
			if tc.window < 0 {
				w = &deadlineContextWriter{
					w: &delayedWriter{}, semaphore: make(chan struct{}, 1), quit: quit,
				}
			} else {
				w = newWriteCoalescer(&delayedWriter{}, 0, tc.window, coalesceFlushThreshold, quit)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := w.writeContext(context.Background(), payload); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func makeCoalesceBenchFrame(size int) []byte {
	buf := make([]byte, size)
	for i := range buf {
		buf[i] = byte(i)
	}
	return buf
}
