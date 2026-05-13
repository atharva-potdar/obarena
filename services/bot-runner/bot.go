package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/coder/websocket"
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

type pendingOrder struct {
	sentAt       time.Time
	expectReject bool
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

func (b *Bot) Run(ctx context.Context, duration time.Duration, ready chan<- struct{}) {
	var conn *websocket.Conn
	for {
		if ctx.Err() != nil {
			return
		}
		var err error
		conn, _, err = websocket.Dial(ctx, b.endpoint, nil)
		if err == nil {
			break
		}
		if strings.Contains(err.Error(), "connection refused") {
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
			continue
		}
		log.Printf("bot %d warmup: %v", b.id, err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	type stepTemplate struct {
		isCancel   bool
		payload    []byte
		tag        string
		cancelTag  string
		expectRej  bool
	}
	templates := make([]stepTemplate, len(b.seq.Steps))
	for i, step := range b.seq.Steps {
		if step.Kind == StepOrder {
			tmpl := fmt.Sprintf(`{"type":"order","order_id":"%%s","side":"%s","order_type":"%s","price":%f,"quantity":%d}`, step.Side, step.OrderType, step.Price, step.Quantity)
			templates[i] = stepTemplate{
				isCancel:  false,
				payload:   []byte(tmpl),
				tag:       step.Tag,
				expectRej: step.ExpectReject,
			}
		} else {
			tmpl := `{"type":"cancel","order_id":"%s"}`
			templates[i] = stepTemplate{
				isCancel:  true,
				payload:   []byte(tmpl),
				cancelTag: step.CancelTag,
			}
		}
	}

	msgCh := make(chan IncomingMessage, 1024)
	type pendingInsert struct {
		oid string
		p   pendingOrder
	}
	pendingCh := make(chan pendingInsert, 1024)
	errCh := make(chan error, 1)

	go func() {
		for {
			_, rawBytes, err := conn.Read(ctx)
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
			var msg IncomingMessage
			if err := json.Unmarshal(rawBytes, &msg); err == nil {
				msgCh <- msg
			}
		}
	}()

	if ready != nil {
		ready <- struct{}{}
	}

	processorCtx, cancelProcessor := context.WithCancel(ctx)
	defer cancelProcessor()
	go func() {
		pending := make(map[string]pendingOrder)
		for {
			select {
			case <-processorCtx.Done():
				return
			case p := <-pendingCh:
				pending[p.oid] = p.p
			case msg := <-msgCh:
				if p, ok := pending[msg.OrderID]; ok {
					switch msg.Type {
					case "ack":
						b.metrics.recordAck(time.Since(p.sentAt))
						b.metrics.acksRecv++
					case "fill":
						b.metrics.recordFill(time.Since(p.sentAt))
						b.metrics.fillsRecv++
						if msg.Remaining == 0 {
							delete(pending, msg.OrderID)
						}
					case "reject":
						b.metrics.rejectsRecv++
						delete(pending, msg.OrderID)
					case "cancel_ack":
						delete(pending, msg.OrderID)
					}
				}
			}
		}
	}()

	deadline := time.Now().Add(duration)
	iteration := 0
	tagToID := make(map[string]string)

	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return
		}
		select {
		case err := <-errCh:
			log.Printf("bot %d read error: %v", b.id, err)
			b.metrics.connDrops++
			return
		default:
		}

		for _, tmpl := range templates {
			var oid string
			var payload []byte

			if tmpl.isCancel {
				targetID, ok := tagToID[tmpl.cancelTag]
				if !ok {
					continue
				}
				oid = targetID
				payload = bytes.Replace(tmpl.payload, []byte("%s"), []byte(targetID), 1)
			} else {
				oid = orderID(b.id, iteration, tmpl.tag)
				tagToID[tmpl.tag] = oid
				payload = bytes.Replace(tmpl.payload, []byte("%s"), []byte(oid), 1)
				
				pendingCh <- pendingInsert{oid, pendingOrder{time.Now(), tmpl.expectRej}}
				b.metrics.ordersSent++
			}

			if err := conn.Write(ctx, websocket.MessageText, payload); err != nil {
				log.Printf("bot %d write error: %v", b.id, err)
				b.metrics.connDrops++
				return
			}
		}
		iteration++
	}
}
