package examples

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/wyfcoding/pkg/client"
)

type KYCClientExample struct {
	factory *client.ClientFactory
}

func NewKYCClientExample(config client.ClientConfig) *KYCClientExample {
	return &KYCClientExample{
		factory: client.NewClientFactory(config),
	}
}

func (e *KYCClientExample) Run(ctx context.Context) error {
	if err := e.factory.Connect(ctx); err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer e.factory.Close()

	fmt.Println("=== KYC Service Client Examples ===\n")

	fmt.Println("1. Creating KYC User")
	userID := fmt.Sprintf("user-%d", time.Now().Unix())
	fmt.Printf("   User ID: %s\n\n", userID)

	fmt.Println("2. Submitting Document for Verification")
	fmt.Printf("   Document Type: passport\n")
	fmt.Printf("   Country: US\n\n")

	fmt.Println("3. Checking Verification Status")
	fmt.Printf("   Status: pending\n\n")

	fmt.Println("4. Performing Face Verification")
	fmt.Printf("   Liveness Score: 0.95\n")
	fmt.Printf("   Similarity Score: 0.92\n\n")

	fmt.Println("5. Risk Assessment")
	riskFactors := map[string]any{
		"country_risk":    "low",
		"document_risk":   "low",
		"behavioral_risk": "medium",
		"overall_score":   25,
	}
	factorsJSON, _ := json.MarshalIndent(riskFactors, "   ", "  ")
	fmt.Printf("   Risk Factors:\n   %s\n\n", string(factorsJSON))

	fmt.Println("6. Audit Trail")
	auditEntry := map[string]any{
		"action":     "document_verified",
		"timestamp":  time.Now().Format(time.RFC3339),
		"operator":   "system",
		"old_status": "pending",
		"new_status": "verified",
	}
	auditJSON, _ := json.MarshalIndent(auditEntry, "   ", "  ")
	fmt.Printf("   Audit Entry:\n   %s\n\n", string(auditJSON))

	return nil
}

type SORClientExample struct {
	factory *client.ClientFactory
}

func NewSORClientExample(config client.ClientConfig) *SORClientExample {
	return &SORClientExample{
		factory: client.NewClientFactory(config),
	}
}

func (e *SORClientExample) Run(ctx context.Context) error {
	if err := e.factory.Connect(ctx); err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer e.factory.Close()

	fmt.Println("=== SOR Service Client Examples ===\n")

	fmt.Println("1. Submitting Smart Order")
	order := map[string]any{
		"symbol":        "AAPL",
		"side":          "buy",
		"quantity":      10000,
		"strategy":      "vwap",
		"duration":      "30m",
		"participation": 0.1,
	}
	orderJSON, _ := json.MarshalIndent(order, "   ", "  ")
	fmt.Printf("   Order:\n   %s\n\n", string(orderJSON))

	fmt.Println("2. Checking Routing Status")
	status := map[string]any{
		"order_id":           "sor-12345",
		"status":             "routing",
		"filled_quantity":    3500,
		"remaining_quantity": 6500,
		"avg_price":          150.25,
		"venues": []map[string]any{
			{"venue": "NYSE", "quantity": 2000, "price": 150.20},
			{"venue": "NASDAQ", "quantity": 1500, "price": 150.30},
		},
	}
	statusJSON, _ := json.MarshalIndent(status, "   ", "  ")
	fmt.Printf("   Status:\n   %s\n\n", string(statusJSON))

	fmt.Println("3. Venue Selection Analysis")
	venues := []map[string]any{
		{"venue": "NYSE", "liquidity": 5000000, "latency_ms": 2, "fee_per_share": 0.003},
		{"venue": "NASDAQ", "liquidity": 4500000, "latency_ms": 3, "fee_per_share": 0.002},
		{"venue": "BATS", "liquidity": 2000000, "latency_ms": 1, "fee_per_share": 0.001},
	}
	for _, v := range venues {
		vJSON, _ := json.MarshalIndent(v, "   ", "  ")
		fmt.Printf("   %s\n", string(vJSON))
	}
	fmt.Println()

	fmt.Println("4. Execution Analytics")
	analytics := map[string]any{
		"arrival_price":   150.00,
		"vwap_price":      150.25,
		"execution_price": 150.22,
		"slippage_bps":    14.67,
		"participation":   0.098,
		"fill_rate":       0.35,
		"market_impact":   0.15,
	}
	analyticsJSON, _ := json.MarshalIndent(analytics, "   ", "  ")
	fmt.Printf("   Analytics:\n   %s\n\n", string(analyticsJSON))

	fmt.Println("5. Strategy Performance")
	strategies := []map[string]any{
		{"strategy": "vwap", "avg_slippage_bps": 12.5, "fill_rate": 0.95},
		{"strategy": "twap", "avg_slippage_bps": 15.2, "fill_rate": 0.92},
		{"strategy": "best_price", "avg_slippage_bps": 8.3, "fill_rate": 0.88},
		{"strategy": "min_impact", "avg_slippage_bps": 10.1, "fill_rate": 0.90},
	}
	for _, s := range strategies {
		sJSON, _ := json.MarshalIndent(s, "   ", "  ")
		fmt.Printf("   %s\n", string(sJSON))
	}
	fmt.Println()

	return nil
}

type SettlementClientExample struct {
	factory *client.ClientFactory
}

func NewSettlementClientExample(config client.ClientConfig) *SettlementClientExample {
	return &SettlementClientExample{
		factory: client.NewClientFactory(config),
	}
}

func (e *SettlementClientExample) Run(ctx context.Context) error {
	if err := e.factory.Connect(ctx); err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer e.factory.Close()

	fmt.Println("=== Settlement Service Client Examples ===\n")

	fmt.Println("1. Creating Settlement Instruction")
	instruction := map[string]any{
		"instruction_id":  "SI-20240101-001",
		"trade_id":        "TRD-12345",
		"security_code":   "AAPL",
		"quantity":        1000,
		"settlement_type": "DVP",
		"settlement_date": "2024-01-03",
		"account_id":      "ACC-001",
		"counterparty":    "ACC-002",
	}
	instructionJSON, _ := json.MarshalIndent(instruction, "   ", "  ")
	fmt.Printf("   Instruction:\n   %s\n\n", string(instructionJSON))

	fmt.Println("2. Netting Process")
	netting := map[string]any{
		"netting_group": "DEFAULT",
		"total_trades":  15,
		"gross_value":   1500000.00,
		"net_value":     850000.00,
		"reduction_pct": 43.33,
	}
	nettingJSON, _ := json.MarshalIndent(netting, "   ", "  ")
	fmt.Printf("   Netting Result:\n   %s\n\n", string(nettingJSON))

	fmt.Println("3. DVP Settlement Flow")
	dvpSteps := []string{
		"1. Validate securities availability",
		"2. Validate cash availability",
		"3. Lock securities in custody",
		"4. Lock cash in account",
		"5. Execute simultaneous transfer",
		"6. Release locked resources",
		"7. Update positions",
	}
	for _, step := range dvpSteps {
		fmt.Printf("   %s\n", step)
	}
	fmt.Println()

	fmt.Println("4. Settlement Status Tracking")
	status := map[string]any{
		"instruction_id":   "SI-20240101-001",
		"status":           "settled",
		"settled_at":       time.Now().Format(time.RFC3339),
		"securities_moved": true,
		"cash_moved":       true,
	}
	statusJSON, _ := json.MarshalIndent(status, "   ", "  ")
	fmt.Printf("   Final Status:\n   %s\n\n", string(statusJSON))

	return nil
}

func RunAllExamples() error {
	ctx := context.Background()

	fmt.Println("========================================")
	fmt.Println("   API Client Examples")
	fmt.Println("========================================\n")

	kycConfig := client.ClientConfig{
		Address: "localhost:9090",
		Timeout: 30 * time.Second,
	}
	kycExample := NewKYCClientExample(kycConfig)
	if err := kycExample.Run(ctx); err != nil {
		fmt.Printf("KYC Example Error: %v\n", err)
	}

	fmt.Println("----------------------------------------\n")

	sorConfig := client.ClientConfig{
		Address: "localhost:9091",
		Timeout: 30 * time.Second,
	}
	sorExample := NewSORClientExample(sorConfig)
	if err := sorExample.Run(ctx); err != nil {
		fmt.Printf("SOR Example Error: %v\n", err)
	}

	fmt.Println("----------------------------------------\n")

	settlementConfig := client.ClientConfig{
		Address: "localhost:9092",
		Timeout: 30 * time.Second,
	}
	settlementExample := NewSettlementClientExample(settlementConfig)
	if err := settlementExample.Run(ctx); err != nil {
		fmt.Printf("Settlement Example Error: %v\n", err)
	}

	return nil
}
