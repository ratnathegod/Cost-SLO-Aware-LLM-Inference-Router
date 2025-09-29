package main

import (
	"testing"
	"time"
)

func TestPercentile(t *testing.T) {
	vals := []int64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	if p := percentile(vals, 0.95); p < 90 || p > 100 {
		t.Fatalf("p95 unexpected: %v", p)
	}
}

func TestTokenBucketApproxRate(t *testing.T) {
	// approximate: ensure that our interval schedule hits near target
	target := 200.0
	interval := time.Duration(float64(time.Second) / target)
	tick := time.NewTicker(interval)
	defer tick.Stop()
	count := 0
	end := time.Now().Add(1 * time.Second)
	for time.Now().Before(end) {
		<-tick.C
		count++
	}
	if count < 150 || count > 250 {
		t.Fatalf("count out of range: %d", count)
	}
}
