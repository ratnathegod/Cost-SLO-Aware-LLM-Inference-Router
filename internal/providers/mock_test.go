package providers

import (
    "testing"
    "time"
)

func TestMockLatencyDistribution(t *testing.T) {
    mp := NewMockProvider(40, 120, 0.0, 0.002)
    var xs []int64
    for i := 0; i < 5000; i++ {
        xs = append(xs, int64(mp.sampleLatency()/time.Millisecond))
    }
    // compute mean and 95th
    var sum float64
    for _, v := range xs { sum += float64(v) }
    mean := sum / float64(len(xs))
    // simple p95
    copyVals := append([]int64(nil), xs...)
    // insertion sort
    for i := 1; i < len(copyVals); i++ {
        j := i
        for j > 0 && copyVals[j-1] > copyVals[j] { copyVals[j-1], copyVals[j] = copyVals[j], copyVals[j-1]; j-- }
    }
    p95 := float64(copyVals[int(float64(len(copyVals))*0.95)-1])
    if mean < 30 || mean > 60 { t.Fatalf("mean out of expected range: %.2f", mean) }
    if p95 < 90 || p95 > 160 { t.Fatalf("p95 out of expected range: %.2f", p95) }
}
