package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type inferReq struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	MaxTok int    `json:"max_tokens,omitempty"`
	Stream bool   `json:"stream,omitempty"`
	Policy string `json:"policy,omitempty"`
}

type inferResp struct {
	Provider  string  `json:"provider"`
	Text      string  `json:"text"`
	CostUSD   float64 `json:"cost_usd"`
	LatencyMs int64   `json:"latency_ms"`
}

type result struct {
	Ts        time.Time
	LatencyMs int64
	Success   bool
	Provider  string
	CostUSD   float64
	Policy    string
	Code      int
}

type summary struct {
	TargetQPS    float64 `json:"target_qps"`
	AchievedQPS  float64 `json:"achieved_qps"`
	Requests     int     `json:"requests"`
	Failures     int     `json:"failures"`
	SuccessRate  float64 `json:"success_rate"`
	ErrorRate    float64 `json:"error_rate"`
	P50          float64 `json:"p50_ms"`
	P90          float64 `json:"p90_ms"`
	P95          float64 `json:"p95_ms"`
	P99          float64 `json:"p99_ms"`
	TotalCostUSD float64 `json:"total_cost_usd"`
}

func percentile(vals []int64, p float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	copyVals := append([]int64(nil), vals...)
	slicesSort(copyVals)
	idx := int(math.Ceil(p*float64(len(copyVals)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(copyVals) {
		idx = len(copyVals) - 1
	}
	return float64(copyVals[idx])
}

func slicesSort(a []int64) {
	// simple insertion sort for small arrays; OK for our modest N
	for i := 1; i < len(a); i++ {
		j := i
		for j > 0 && a[j-1] > a[j] {
			a[j-1], a[j] = a[j], a[j-1]
			j--
		}
	}
}

func main() {
	baseURL := flag.String("base-url", "http://localhost:8080", "server base URL")
	duration := flag.Duration("duration", 60*time.Second, "test duration")
	qps := flag.Float64("qps", 100, "target QPS")
	conc := flag.Int("concurrency", 32, "number of workers")
	policy := flag.String("policy", "", "policy to use (empty=server default)")
	model := flag.String("model", "", "model to use (empty=server default)")
	prompt := flag.String("prompt", "", "inline prompt")
	promptFile := flag.String("prompt-file", "", "file with prompt content")
	maxTok := flag.Int("max-tokens", 64, "max tokens")
	timeout := flag.Duration("timeout", 5*time.Second, "per-request timeout")
	warmup := flag.Duration("warmup", 5*time.Second, "warmup duration, excluded from metrics")
	csvOut := flag.String("csv-out", "", "path to write CSV results")
	jsonSummary := flag.String("json-summary", "", "path to write JSON summary (stdout if empty)")
	flag.Parse()

	if *prompt != "" && *promptFile != "" {
		fmt.Fprintln(os.Stderr, "--prompt and --prompt-file are mutually exclusive")
		os.Exit(2)
	}
	var ptxt string
	if *promptFile != "" {
		b, err := os.ReadFile(*promptFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		ptxt = string(b)
	} else {
		ptxt = *prompt
		if ptxt == "" {
			ptxt = "ping"
		}
	}

	client := &http.Client{Timeout: *timeout}
	inferURL := strings.TrimRight(*baseURL, "/") + "/v1/infer"
	readyURL := strings.TrimRight(*baseURL, "/") + "/v1/readyz"

	// Warmup: poll readyz
	readyDeadline := time.Now().Add(*warmup)
	for time.Now().Before(readyDeadline) {
		resp, err := client.Get(readyURL)
		if err == nil && resp.StatusCode == 200 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Token bucket (simple)
	interval := time.Duration(float64(time.Second) / *qps)
	if interval <= 0 {
		interval = time.Millisecond
	}
	var tokens int64
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	resCh := make(chan result, *conc*16)
	var wg sync.WaitGroup
	var sent int64

	worker := func(id int) {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				atomic.AddInt64(&tokens, 1)
			default:
			}
			if atomic.LoadInt64(&tokens) <= 0 {
				time.Sleep(100 * time.Microsecond)
				continue
			}
			atomic.AddInt64(&tokens, -1)
			atomic.AddInt64(&sent, 1)
			// fire request
			go func() {
				start := time.Now()
				reqBody := inferReq{Model: *model, Prompt: ptxt, MaxTok: *maxTok, Policy: *policy}
				b, _ := json.Marshal(reqBody)
				req, _ := http.NewRequestWithContext(ctx, http.MethodPost, inferURL, strings.NewReader(string(b)))
				req.Header.Set("Content-Type", "application/json")
				resp, err := client.Do(req)
				lat := time.Since(start).Milliseconds()
				if err != nil {
					resCh <- result{Ts: time.Now(), LatencyMs: lat, Success: false, Provider: "", CostUSD: 0, Policy: *policy, Code: 0}
					return
				}
				defer resp.Body.Close()
				if resp.StatusCode != 200 {
					io.Copy(io.Discard, resp.Body)
					resCh <- result{Ts: time.Now(), LatencyMs: lat, Success: false, Provider: "", CostUSD: 0, Policy: *policy, Code: resp.StatusCode}
					return
				}
				var ir inferResp
				if err := json.NewDecoder(resp.Body).Decode(&ir); err != nil {
					resCh <- result{Ts: time.Now(), LatencyMs: lat, Success: false, Provider: "", CostUSD: 0, Policy: *policy, Code: 200}
					return
				}
				resCh <- result{Ts: time.Now(), LatencyMs: lat, Success: true, Provider: ir.Provider, CostUSD: ir.CostUSD, Policy: *policy, Code: 200}
			}()
		}
	}

	wg.Add(*conc)
	for i := 0; i < *conc; i++ {
		go worker(i)
	}

	go func() {
		wg.Wait()
		close(resCh)
	}()

	// Collect results live
	t0 := time.Now()
	var latencies []int64
	var totalCost float64
	var reqs, fails int
	var lastTick = time.Now()

	var csvWriter *csv.Writer
	if *csvOut != "" {
		f, err := os.Create(*csvOut)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer f.Close()
		csvWriter = csv.NewWriter(f)
		defer csvWriter.Flush()
		csvWriter.Write([]string{"ts", "latency_ms", "success", "provider", "cost_usd", "policy", "code"})
	}

	progress := time.NewTicker(time.Second)
	defer progress.Stop()

	for {
		select {
		case r, ok := <-resCh:
			if !ok {
				goto done
			}
			if time.Since(t0) < *warmup {
				continue
			}
			reqs++
			if !r.Success {
				fails++
			}
			latencies = append(latencies, r.LatencyMs)
			totalCost += r.CostUSD
			if csvWriter != nil {
				csvWriter.Write([]string{r.Ts.Format(time.RFC3339Nano), fmt.Sprintf("%d", r.LatencyMs), fmt.Sprintf("%t", r.Success), r.Provider, fmt.Sprintf("%.6f", r.CostUSD), r.Policy, fmt.Sprintf("%d", r.Code)})
			}
		case <-progress.C:
			_ = time.Since(lastTick).Seconds()
			lastTick = time.Now()
			achieved := float64(reqs) / math.Max(1e-9, time.Since(t0).Seconds())
			fmt.Printf("qps=%.1f reqs=%d fails=%d\n", achieved, reqs, fails)
		case <-ctx.Done():
			goto done
		}
	}

done:
	elapsed := time.Since(t0).Seconds() - math.Max(0, warmup.Seconds())
	achieved := float64(reqs) / math.Max(1e-9, elapsed)
	s := summary{
		TargetQPS:    *qps,
		AchievedQPS:  achieved,
		Requests:     reqs,
		Failures:     fails,
		SuccessRate:  float64(reqs-fails) / math.Max(1, float64(reqs)),
		ErrorRate:    float64(fails) / math.Max(1, float64(reqs)),
		P50:          percentile(latencies, 0.50),
		P90:          percentile(latencies, 0.90),
		P95:          percentile(latencies, 0.95),
		P99:          percentile(latencies, 0.99),
		TotalCostUSD: totalCost,
	}
	if *jsonSummary != "" {
		b, _ := json.MarshalIndent(s, "", "  ")
		if err := os.WriteFile(*jsonSummary, b, 0644); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	} else {
		b, _ := json.MarshalIndent(s, "", "  ")
		fmt.Println(string(b))
	}
	// Exit code per spec
	if s.ErrorRate > 0.01 || s.AchievedQPS < 0.90*(*qps) {
		os.Exit(1)
	}
}
