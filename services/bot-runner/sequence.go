package main

import (
	"fmt"
)

type StepKind int

const (
	StepOrder StepKind = iota
	StepCancel
)

type Step struct {
	Kind      StepKind
	Side      string
	OrderType string
	Price     float64
	Quantity  int64
	Tag       string
	CancelTag string

	// Correctness assertions (used only by the synchronous correctness bot).
	// Load bots ignore these fields entirely.
	ExpectFill      bool    // expect a fill message for this order
	ExpectFillPrice float64 // expected fill price (0 = don't check price)
	ExpectFillQty   int64   // expected filled quantity (0 = don't check)
	ExpectReject    bool    // expect a reject message
	ExpectBookBids  int     // expected bid levels after this step (-1 = don't check)
	ExpectBookAsks  int     // expected ask levels after this step (-1 = don't check)
}

type Sequence struct {
	Name  string
	Steps []Step
}

func orderID(botID int, iteration int, tag string) string {
	return fmt.Sprintf("b%d-i%d-%s", botID, iteration, tag)
}

// basePrice gives each bot a unique price band 1000 units apart.
// Bot 0 trades at 1000, bot 1 at 2000, etc.
// Bands are wide enough that ladder sequences (±2) never overlap.
func basePrice(botID int) float64 {
	return float64((botID + 1) * 1000)
}

// ── Load-test sequences (no assertions, loop continuously) ──────────────

func sequences(botID int) []Sequence {
	return []Sequence{
		basicMatch(botID),
		partialFillLadder(botID),
		cancelCorrectness(botID),
	}
}

func basicMatch(botID int) Sequence {
	p := basePrice(botID)
	return Sequence{
		Name: "basic_match",
		Steps: []Step{
			{
				Kind:      StepOrder,
				Tag:       "bid1",
				Side:      "buy",
				OrderType: "limit",
				Price:     p,
				Quantity:  10,
			},
			{
				Kind:      StepOrder,
				Tag:       "ask1",
				Side:      "sell",
				OrderType: "limit",
				Price:     p,
				Quantity:  10,
			},
		},
	}
}

func partialFillLadder(botID int) Sequence {
	p := basePrice(botID)
	return Sequence{
		Name: "partial_fill_ladder",
		Steps: []Step{
			{
				Kind:      StepOrder,
				Tag:       "bid-0",
				Side:      "buy",
				OrderType: "limit",
				Price:     p,
				Quantity:  5,
			},
			{
				Kind:      StepOrder,
				Tag:       "bid-1",
				Side:      "buy",
				OrderType: "limit",
				Price:     p - 1,
				Quantity:  5,
			},
			{
				Kind:      StepOrder,
				Tag:       "bid-2",
				Side:      "buy",
				OrderType: "limit",
				Price:     p - 2,
				Quantity:  5,
			},
			{
				Kind:      StepOrder,
				Tag:       "sweep",
				Side:      "sell",
				OrderType: "market",
				Quantity:  15,
			},
		},
	}
}

func cancelCorrectness(botID int) Sequence {
	p := basePrice(botID)
	return Sequence{
		Name: "cancel_correctness",
		Steps: []Step{
			{
				Kind:      StepOrder,
				Tag:       "resting",
				Side:      "buy",
				OrderType: "limit",
				Price:     p,
				Quantity:  10,
			},
			{
				Kind:      StepCancel,
				CancelTag: "resting",
			},
			{
				Kind:      StepOrder,
				Tag:       "probe",
				Side:      "sell",
				OrderType: "limit",
				Price:     p,
				Quantity:  10,
			},
			{
				Kind:      StepCancel,
				CancelTag: "probe",
			},
		},
	}
}

// ── Correctness sequences (per-step assertions, run once) ───────────────

// correctnessBasePrice is well below load-bot bands (which start at 1000).
const correctnessBasePrice = 500.0

// CorrectnessSequences returns the deterministic sequences used by the
// synchronous correctness bot. Each runs exactly once with per-step assertions.
func CorrectnessSequences() []Sequence {
	return []Sequence{
		cxBasicMatch(),
		cxPriorityFill(),
		cxPartialFill(),
		cxCancel(),
		cxRejectMarketNoLiquidity(),
	}
}

// cxBasicMatch: buy then sell at the same price, expect a fill.
func cxBasicMatch() Sequence {
	p := correctnessBasePrice
	return Sequence{
		Name: "cx_basic_match",
		Steps: []Step{
			{
				Kind: StepOrder, Tag: "bid", Side: "buy",
				OrderType: "limit", Price: p, Quantity: 10,
				ExpectBookBids: 1, ExpectBookAsks: -1,
			},
			{
				Kind: StepOrder, Tag: "ask", Side: "sell",
				OrderType: "limit", Price: p, Quantity: 10,
				ExpectFill: true, ExpectFillPrice: p, ExpectFillQty: 10,
				ExpectBookBids: 0, ExpectBookAsks: 0,
			},
		},
	}
}

// cxPriorityFill: two bids at different prices; sell should fill the
// higher-priced bid first (price priority).
func cxPriorityFill() Sequence {
	p := correctnessBasePrice
	return Sequence{
		Name: "cx_priority_fill",
		Steps: []Step{
			{
				Kind: StepOrder, Tag: "bid-lo", Side: "buy",
				OrderType: "limit", Price: p, Quantity: 5,
				ExpectBookBids: 1, ExpectBookAsks: -1,
			},
			{
				Kind: StepOrder, Tag: "bid-hi", Side: "buy",
				OrderType: "limit", Price: p + 1, Quantity: 5,
				ExpectBookBids: 2, ExpectBookAsks: -1,
			},
			{
				Kind: StepOrder, Tag: "sell", Side: "sell",
				OrderType: "limit", Price: p, Quantity: 10,
				ExpectFill: true, ExpectFillPrice: p + 1,
				ExpectBookBids: 0, ExpectBookAsks: 0,
			},
		},
	}
}

// cxPartialFill: buy 10, sell 3 → partial fill, 7 remaining on the bid.
func cxPartialFill() Sequence {
	p := correctnessBasePrice + 10
	return Sequence{
		Name: "cx_partial_fill",
		Steps: []Step{
			{
				Kind: StepOrder, Tag: "bid", Side: "buy",
				OrderType: "limit", Price: p, Quantity: 10,
				ExpectBookBids: 1, ExpectBookAsks: -1,
			},
			{
				Kind: StepOrder, Tag: "ask", Side: "sell",
				OrderType: "limit", Price: p, Quantity: 3,
				ExpectFill: true, ExpectFillPrice: p, ExpectFillQty: 3,
				ExpectBookBids: 1, ExpectBookAsks: 0,
			},
			// Clean up: cancel the remaining bid
			{
				Kind: StepCancel, CancelTag: "bid",
				ExpectBookBids: 0, ExpectBookAsks: 0,
			},
		},
	}
}

// cxCancel: place, cancel, verify empty book, then verify sell doesn't match.
func cxCancel() Sequence {
	p := correctnessBasePrice + 20
	return Sequence{
		Name: "cx_cancel",
		Steps: []Step{
			{
				Kind: StepOrder, Tag: "bid", Side: "buy",
				OrderType: "limit", Price: p, Quantity: 10,
				ExpectBookBids: 1, ExpectBookAsks: -1,
			},
			{
				Kind: StepCancel, CancelTag: "bid",
				ExpectBookBids: 0, ExpectBookAsks: 0,
			},
			{
				Kind: StepOrder, Tag: "ask", Side: "sell",
				OrderType: "limit", Price: p, Quantity: 10,
				ExpectBookBids: 0, ExpectBookAsks: 1,
			},
			{
				Kind: StepCancel, CancelTag: "ask",
				ExpectBookBids: 0, ExpectBookAsks: 0,
			},
		},
	}
}

// cxRejectMarketNoLiquidity: market sell on empty book → reject.
func cxRejectMarketNoLiquidity() Sequence {
	return Sequence{
		Name: "cx_reject_market_no_liquidity",
		Steps: []Step{
			{
				Kind: StepOrder, Tag: "mkt", Side: "sell",
				OrderType: "market", Quantity: 10,
				ExpectReject:   true,
				ExpectBookBids: 0, ExpectBookAsks: 0,
			},
		},
	}
}
