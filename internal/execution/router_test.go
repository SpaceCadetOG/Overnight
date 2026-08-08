package execution

import "testing"

type mockExecutor struct{}

func (m mockExecutor) Submit(OrderRequest) (OrderResponse, error) {
	return OrderResponse{}, nil
}

func (m mockExecutor) Cancel(string) error {
	return nil
}

func (m mockExecutor) GetPosition(string) float64 {
	return 0
}

func TestRouterModes(t *testing.T) {

	router := NewRouter(
		mockExecutor{},
		mockExecutor{},
	)

	tests := []struct {
		symbol string
		mode   Mode
	}{
		{"BTC", Live},
		{"ETH", Live},
		{"SOL", Paper},
		{"XAU", Paper},
	}

	for _, tt := range tests {

		_, mode, err := router.Executor(tt.symbol)

		if err != nil {
			t.Fatal(err)
		}

		if mode != tt.mode {
			t.Fatalf(
				"%s expected %s got %s",
				tt.symbol,
				tt.mode,
				mode,
			)
		}
	}
}

func (m mockExecutor) Close(
	symbol string,
	side string,
	size float64,
	price float64,
) (OrderResponse, error) {

	return OrderResponse{
		OrderID: "mock-close",
		Status:  "CLOSED",
		Mode:    Paper,
	}, nil
}
