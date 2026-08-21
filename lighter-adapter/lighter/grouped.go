package lighter

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/elliottech/lighter-go/types"
	"github.com/elliottech/lighter-go/types/txtypes"
)

type GroupingMode uint8

const (
	GroupingOTO   GroupingMode = GroupingMode(txtypes.GroupingType_OneTriggersTheOther)
	GroupingOCO   GroupingMode = GroupingMode(txtypes.GroupingType_OneCancelsTheOther)
	GroupingOTOCO GroupingMode = GroupingMode(txtypes.GroupingType_OneTriggersAOneCancelsTheOther)
)

type GroupedOrderPreview struct {
	GroupingMode GroupingMode      `json:"grouping_mode"`
	Nonce        int64             `json:"nonce"`
	TxType       uint8             `json:"tx_type"`
	TxInfo       string            `json:"tx_info"`
	Orders       []OrderSubmission `json:"orders"`
}

func validateGroupingMode(mode GroupingMode, count int) error {
	if mode != GroupingOTO && mode != GroupingOCO && mode != GroupingOTOCO {
		return fmt.Errorf("unsupported grouping mode %d", mode)
	}
	if count < 2 || count > int(txtypes.MaxGroupedOrderCount) {
		return fmt.Errorf("grouped order count must be between 2 and %d", txtypes.MaxGroupedOrderCount)
	}
	return nil
}

// PreviewGroupedOrders builds, signs, validates, and exposes tx_info for
// inspection without submitting it. Funded runtime must not call this as an
// execution primitive until portfolio checks and exchange semantics have a
// separately approved end-to-end test.
func (e *ExecutionEngine) PreviewGroupedOrders(ctx context.Context, mode GroupingMode, requests []PlaceOrderRequest) (*GroupedOrderPreview, error) {
	if err := validateGroupingMode(mode, len(requests)); err != nil {
		return nil, err
	}
	if e.manager.TxClient == nil {
		return nil, errors.New("transaction client is not configured")
	}
	orders := make([]*types.CreateOrderTxReq, 0, len(requests))
	evidence := make([]OrderSubmission, 0, len(requests))
	for _, request := range requests {
		if strings.TrimSpace(request.IntentKey) == "" || request.ClientOrderIndex <= 0 {
			return nil, errors.New("grouped orders require deterministic intent and client order indexes")
		}
		orderType, tif, expiry, err := encodeOrderOptions(request)
		if err != nil {
			return nil, err
		}
		market, err := e.manager.MarketBySymbol(ctx, request.Symbol)
		if err != nil {
			return nil, err
		}
		encoded, err := market.EncodeOrder(request.Quantity, request.Price)
		if err != nil {
			return nil, err
		}
		trigger := uint32(0)
		if request.TriggerPrice > 0 {
			trigger, err = market.EncodePrice(request.TriggerPrice)
			if err != nil {
				return nil, err
			}
		}
		isAsk, reduceOnly := uint8(0), uint8(0)
		if request.Side == SideSell {
			isAsk = 1
		} else if request.Side != SideBuy {
			return nil, fmt.Errorf("invalid side %q", request.Side)
		}
		if request.ReduceOnly {
			reduceOnly = 1
		}
		orders = append(orders, &types.CreateOrderTxReq{
			MarketIndex: encoded.MarketIndex, ClientOrderIndex: request.ClientOrderIndex,
			BaseAmount: encoded.BaseAmount, Price: encoded.Price, IsAsk: isAsk,
			Type: orderType, TimeInForce: tif, ReduceOnly: reduceOnly,
			TriggerPrice: trigger, OrderExpiry: expiry,
		})
		evidence = append(evidence, OrderSubmission{
			IntentKey: request.IntentKey, ClientOrderIndex: request.ClientOrderIndex,
			MarketIndex: encoded.MarketIndex, EncodedBaseAmount: encoded.BaseAmount,
			EncodedPrice: encoded.Price, RequestedQuantity: request.Quantity, RequestedPrice: request.Price,
		})
	}
	e.nonceMu.RLock()
	nonce := e.nonces.Peek()
	e.nonceMu.RUnlock()
	tx, err := e.manager.TxClient.GetCreateGroupedOrdersTransaction(
		&types.CreateGroupedOrdersTxReq{GroupingType: uint8(mode), Orders: orders},
		&types.TransactOpts{Nonce: &nonce},
	)
	if err != nil {
		return nil, fmt.Errorf("build grouped transaction: %w", err)
	}
	if err := tx.Validate(); err != nil {
		return nil, fmt.Errorf("validate grouped transaction: %w", err)
	}
	txInfo, err := tx.GetTxInfo()
	if err != nil {
		return nil, fmt.Errorf("encode grouped transaction: %w", err)
	}
	for i := range evidence {
		evidence[i].Nonce = nonce
	}
	return &GroupedOrderPreview{GroupingMode: mode, Nonce: nonce, TxType: tx.GetTxType(), TxInfo: txInfo, Orders: evidence}, nil
}
