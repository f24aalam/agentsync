# Sample Table-Driven Test Pattern

Use this template when authoring unit tests for business logic in Go.

```go
package service_test

import (
	"context"
	"testing"
)

func TestProcessOrder(t *testing.T) {
	tests := []struct {
		name    string
		orderID string
		wantErr bool
	}{
		{
			name:    "valid order ID processes successfully",
			orderID: "ord-12345",
			wantErr: false,
		},
		{
			name:    "empty order ID returns validation error",
			orderID: "",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ProcessOrder(context.Background(), tc.orderID)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ProcessOrder() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
```
