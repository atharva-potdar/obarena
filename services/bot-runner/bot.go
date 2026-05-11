package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type IncomingMessage struct {
	Type      string  `json:"type"`
	OrderID   string  `json:"order_id"`
	Reason    string  `json:"reason"`
	FilledQty int64   `json:"filled_qty"`
	FillPrice float64 `json:"fill_price"`
	Remaining int64   `json:"remaining"`
	Timestamp int64   `json:"timestamp"`
}

type OutgoingMessage struct {
	Type      string  `json:"type"`
	OrderID   string  `json:"order_id"`
	Side      string  `json:"side,omitempty"`
	OrderType string  `json:"order_type,omitempty"`
	Price     float64 `json:"price,omitempty"`
	Quantity  int64   `json:"quantity,omitempty"`
}

type pendingOrder struct {
	sentAt time.Time
	tag    string
}

type Bot struct {
	id       int
	endpoint string
	seq      Sequence
	metrics  *BotMetrics
}

func NewBot(id int, endpoint string, seq Sequence) *Bot {
	return &Bot{
		id:       id,
		endpoint: endpoint,
		seq:      seq,
		metrics:  NewBotMetrics(),
	}
}

func (b *Bot) Run(ctx context.Context, duration time.Duration) {
	deadline := time.Now().Add(duration)
	iteration := 0

	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return
		}
		if err := b.runIteration(ctx, iteration); err != nil {
			log.Printf("bot %d iter %d: %v", b.id, iteration, err)
			b.metrics.connDrops++
			// Brief backoff before reconnecting
			select {
			case <-ctx.Done():
				return
			case <-time.After(50 * time.Millisecond):
			}
		}
		iteration++
	}
}

func (b *Bot) runIteration(ctx context.Context, iteration int) error {
	conn, _, err := websocket.Dial(ctx, b.endpoint, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	// pending maps order_id → pendingOrder for in-flight orders
	pending := make(map[string]pendingOrder, len(b.seq.Steps))
	// tagToID maps step tag → resolved order_id for cancel lookups
	tagToID := make(map[string]string, len(b.seq.Steps))

	recvCh := make(chan IncomingMessage, 64)
	errCh := make(chan error, 1)

	// Read loop — runs concurrently with the send loop
	go func() {
		for {
			var msg IncomingMessage
			if err := wsjson.Read(ctx, conn, &msg); err != nil {
				errCh <- err
				return
			}
			recvCh <- msg
		}
	}()

	for i, step := range b.seq.Steps {
		switch step.Kind {
		case StepOrder:
			oid := orderID(b.id, iteration, step.Tag)
			tagToID[step.Tag] = oid

			out := OutgoingMessage{
				Type:      "order",
				OrderID:   oid,
				Side:      step.Side,
				OrderType: step.OrderType,
				Price:     step.Price,
				Quantity:  step.Quantity,
			}
			sentAt := time.Now()
			if err := wsjson.Write(ctx, conn, out); err != nil {
				return fmt.Errorf("write step %d: %w", i, err)
			}
			b.metrics.ordersSent++
			pending[oid] = pendingOrder{sentAt: sentAt, tag: step.Tag}

			// Collect expected responses for this step
			if err := b.collectResponses(ctx, step, oid, sentAt, pending, recvCh, errCh); err != nil {
				return err
			}

		case StepCancel:
			oid, ok := tagToID[step.CancelTag]
			if !ok {
				return fmt.Errorf("cancel: unknown tag %q", step.CancelTag)
			}
			out := OutgoingMessage{
				Type:    "cancel",
				OrderID: oid,
			}
			if err := wsjson.Write(ctx, conn, out); err != nil {
				return fmt.Errorf("write cancel step %d: %w", i, err)
			}
			// Wait for cancel_ack
			if err := b.waitForCancelAck(ctx, oid, recvCh, errCh); err != nil {
				return err
			}
		}
	}

	return nil
}

func (b *Bot) collectResponses(
	ctx context.Context,
	step Step,
	oid string,
	sentAt time.Time,
	pending map[string]pendingOrder,
	recvCh <-chan IncomingMessage,
	errCh <-chan error,
) error {
	gotAck := !step.ExpectAck
	gotFill := !step.ExpectFill
	gotReject := !step.ExpectReject

	timeout := time.After(5 * time.Second)

	for !gotAck || !gotFill || !gotReject {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			return fmt.Errorf("read: %w", err)
		case <-timeout:
			return fmt.Errorf("timeout waiting for responses to order %s", oid)
		case msg := <-recvCh:
			if msg.OrderID != oid {
				// Not for this order — could be a fill for a previously
				// resting order. Record fill latency if we have it pending.
				if p, ok := pending[msg.OrderID]; ok && msg.Type == "fill" {
					b.metrics.recordFill(time.Since(p.sentAt))
				}
				continue
			}
			switch msg.Type {
			case "ack":
				b.metrics.recordAck(time.Since(sentAt))
				b.metrics.acksRecv++
				gotAck = true
			case "fill":
				b.metrics.recordFill(time.Since(sentAt))
				b.metrics.fillsRecv++
				if msg.Remaining == 0 {
					gotFill = true
					delete(pending, oid)
				}
			case "reject":
				b.metrics.rejectsRecv++
				if step.ExpectReject && msg.Reason == step.RejectReason {
					gotReject = true
				} else if !step.ExpectReject {
					return fmt.Errorf("unexpected reject for %s: %s", oid, msg.Reason)
				}
				gotFill = true // reject is terminal, no fill coming
				delete(pending, oid)
			}
		}
	}
	return nil
}

func (b *Bot) waitForCancelAck(
	ctx context.Context,
	oid string,
	recvCh <-chan IncomingMessage,
	errCh <-chan error,
) error {
	timeout := time.After(5 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			return fmt.Errorf("read: %w", err)
		case <-timeout:
			return fmt.Errorf("timeout waiting for cancel_ack for %s", oid)
		case msg := <-recvCh:
			if msg.OrderID == oid && msg.Type == "cancel_ack" {
				return nil
			}
		}
	}
}
