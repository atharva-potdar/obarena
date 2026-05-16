package main

import (
	"fmt"
	"time"

	hdrhistogram "github.com/HdrHistogram/hdrhistogram-go"
)

const (
	minLatencyUS = 1
	maxLatencyUS = 60 * 1000 * 1000 // 60 seconds in microseconds
	sigFigures   = 3
)

type BotMetrics struct {
	ackLatency  *hdrhistogram.Histogram
	fillLatency *hdrhistogram.Histogram
	ordersSent  int64
	acksRecv    int64
	fillsRecv   int64
	rejectsRecv int64
	connDrops   int64
}

func NewBotMetrics() *BotMetrics {
	return &BotMetrics{
		ackLatency:  hdrhistogram.New(minLatencyUS, maxLatencyUS, sigFigures),
		fillLatency: hdrhistogram.New(minLatencyUS, maxLatencyUS, sigFigures),
	}
}

func (m *BotMetrics) recordAck(d time.Duration) {
	_ = m.ackLatency.RecordValue(d.Microseconds())
	m.acksRecv++
}

func (m *BotMetrics) recordFill(d time.Duration) {
	_ = m.fillLatency.RecordValue(d.Microseconds())
	m.fillsRecv++
}

type AggregateMetrics struct {
	ackLatency  *hdrhistogram.Histogram
	fillLatency *hdrhistogram.Histogram
	ordersSent  int64
	acksRecv    int64
	fillsRecv   int64
	rejectsRecv int64
	connDrops   int64
	ackDropped  int64
	fillDropped int64
}

func merge(bots []*BotMetrics) *AggregateMetrics {
	agg := &AggregateMetrics{
		ackLatency:  hdrhistogram.New(minLatencyUS, maxLatencyUS, sigFigures),
		fillLatency: hdrhistogram.New(minLatencyUS, maxLatencyUS, sigFigures),
	}
	for _, b := range bots {
		agg.ackDropped += agg.ackLatency.Merge(b.ackLatency)
		agg.fillDropped += agg.fillLatency.Merge(b.fillLatency)
		agg.ordersSent += b.ordersSent
		agg.acksRecv += b.acksRecv
		agg.fillsRecv += b.fillsRecv
		agg.rejectsRecv += b.rejectsRecv
		agg.connDrops += b.connDrops
	}
	return agg
}

func report(agg *AggregateMetrics, duration time.Duration) {
	tps := float64(agg.ordersSent) / duration.Seconds()
	fmt.Println("=== Bot Runner Report ===")
	fmt.Printf("Duration:      %s\n", duration.Round(time.Millisecond))
	fmt.Printf("Orders sent:   %d (%.0f TPS)\n", agg.ordersSent, tps)
	fmt.Printf("Acks received: %d\n", agg.acksRecv)
	fmt.Printf("Fills received:%d\n", agg.fillsRecv)
	fmt.Printf("Rejects:       %d\n", agg.rejectsRecv)
	fmt.Printf("Conn drops:    %d\n", agg.connDrops)
	fmt.Printf("Hist dropped:  ack=%d fill=%d\n", agg.ackDropped, agg.fillDropped)
	fmt.Println()
	fmt.Println("--- Ack Latency (µs) ---")
	printHistogram(agg.ackLatency)
	fmt.Println()
	fmt.Println("--- Fill Latency (µs) ---")
	printHistogram(agg.fillLatency)
}

func printHistogram(h *hdrhistogram.Histogram) {
	fmt.Printf("  p50:  %d µs\n", h.ValueAtQuantile(50))
	fmt.Printf("  p90:  %d µs\n", h.ValueAtQuantile(90))
	fmt.Printf("  p99:  %d µs\n", h.ValueAtQuantile(99))
	fmt.Printf("  p999: %d µs\n", h.ValueAtQuantile(99.9))
	fmt.Printf("  max:  %d µs\n", h.Max())
}
