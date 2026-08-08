package lighter

import (
	"fmt"

	lighterclient "github.com/elliottech/lighter-go/client"
	lightertypes "github.com/elliottech/lighter-go/types"
	"github.com/elliottech/lighter-go/types/txtypes"
)

type Signer struct {
	client *lighterclient.TxClient
}

func NewSigner(
	httpClient lighterclient.MinimalHTTPClient,
	privateKey string,
	accountIndex int64,
	apiKeyIndex uint8,
	chainID uint32,
) (*Signer, error) {

	txClient, err := lighterclient.NewTxClient(
		httpClient,
		privateKey,
		accountIndex,
		apiKeyIndex,
		chainID,
	)

	if err != nil {
		return nil, err
	}

	return &Signer{
		client: txClient,
	}, nil
}

func (s *Signer) SignCreateOrder(
	req *lightertypes.CreateOrderTxReq,
	opts *lightertypes.TransactOpts,
) (*txtypes.L2CreateOrderTxInfo, error) {

	if s.client == nil {
		return nil, fmt.Errorf("lighter signer not initialized")
	}

	return s.client.GetCreateOrderTransaction(
		req,
		opts,
	)
}

func (s *Signer) SignCancelOrder(
	req *lightertypes.CancelOrderTxReq,
	opts *lightertypes.TransactOpts,
) (*txtypes.L2CancelOrderTxInfo, error) {

	if s.client == nil {
		return nil, fmt.Errorf("lighter signer not initialized")
	}

	return s.client.GetCancelOrderTransaction(
		req,
		opts,
	)
}

func (s *Signer) SignModifyOrder(
	req *lightertypes.ModifyOrderTxReq,
	opts *lightertypes.TransactOpts,
) (*txtypes.L2ModifyOrderTxInfo, error) {

	if s.client == nil {
		return nil, fmt.Errorf("lighter signer not initialized")
	}

	return s.client.GetModifyOrderTransaction(
		req,
		opts,
	)
}

func (s *Signer) Check() error {

	if s.client == nil {
		return fmt.Errorf("missing lighter client")
	}

	return s.client.Check()
}
