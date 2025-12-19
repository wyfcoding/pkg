package algorithm

import (
	"testing"
	"time"
)

func TestPricingEngine(t *testing.T) {
	basePrice := int64(10000) // 100.00
	minPrice := int64(8000)   // 80.00
	maxPrice := int64(15000)  // 150.00
	elasticity := 1.2

	pe := NewPricingEngine(basePrice, minPrice, maxPrice, elasticity)

	factors := PricingFactors{
		Stock:           100,
		TotalStock:      1000,
		DemandLevel:     0.8,
		CompetitorPrice: 9500,
		TimeOfDay:       14,
		DayOfWeek:       3,
		IsHoliday:       false,
		UserLevel:       1,
		SeasonFactor:    1.0,
	}

	price := pe.CalculatePrice(factors)
	if price < minPrice || price > maxPrice {
		t.Errorf("Price %d out of range [%d, %d]", price, minPrice, maxPrice)
	}
}

func TestCouponOptimizer(t *testing.T) {
	optimizer := NewCouponOptimizer()

	coupons := []Coupon{
		{ID: 1, Type: CouponTypeDiscount, Threshold: 10000, DiscountRate: 0.8, Priority: 1},     // 20% off > 100
		{ID: 2, Type: CouponTypeReduction, Threshold: 5000, ReductionAmount: 1000, Priority: 1}, // -10 > 50
	}

	orderAmount := int64(20000) // 200.00
	bestCombination, finalPrice, discount := optimizer.OptimalCombination(orderAmount, coupons)

	if finalPrice >= orderAmount {
		t.Errorf("Expected discount, got final price %d", finalPrice)
	}
	if discount <= 0 {
		t.Errorf("Expected positive discount, got %d", discount)
	}
	if len(bestCombination) == 0 {
		t.Errorf("Expected coupons to be used")
	}
}

func TestAntiBotDetector(t *testing.T) {
	detector := NewAntiBotDetector()

	behavior := UserBehavior{
		UserID:    1,
		IP:        "127.0.0.1",
		Timestamp: time.Now(),
		Action:    "login",
	}

	isBot, _ := detector.IsBot(behavior)
	if isBot {
		t.Errorf("Expected normal behavior, got bot")
	}

	// Simulate bot
	for i := 0; i < 30; i++ {
		detector.IsBot(UserBehavior{
			UserID:    1,
			IP:        "127.0.0.1",
			Timestamp: time.Now(),
			Action:    "login",
		})
	}

	isBot, reason := detector.IsBot(behavior)
	if !isBot {
		t.Errorf("Expected bot detection due to frequency")
	} else {
		t.Logf("Bot detected as expected: %s", reason)
	}
}
