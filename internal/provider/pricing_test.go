package provider

import (
	"math"
	"testing"
)

func TestGetPricing(t *testing.T) {
	cases := []struct {
		name       string
		provider   string
		wantInput  float64
		wantOutput float64
	}{
		{"anthropic", "anthropic", 3.0, 15.0},
		{"openai", "openai", 2.5, 10.0},
		{"deepseek", "deepseek", 0.14, 0.28},
		{"nvidia-free", "nvidia", 0.0, 0.0},
		{"unknown", "unknown-provider", 0.0, 0.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := GetPricing(tc.provider)
			if p.InputPer1K != tc.wantInput {
				t.Errorf("InputPer1K = %v, want %v", p.InputPer1K, tc.wantInput)
			}
			if p.OutputPer1K != tc.wantOutput {
				t.Errorf("OutputPer1K = %v, want %v", p.OutputPer1K, tc.wantOutput)
			}
		})
	}
}

func TestCalculateCost(t *testing.T) {
	cases := []struct {
		name         string
		pricing      Pricing
		inputTokens  int
		outputTokens int
		want         float64
	}{
		{"zero-pricing", Pricing{0, 0}, 1000, 500, 0},
		{"anthropic", Pricing{3.0, 15.0}, 1000, 500, 3.0*1.0 + 15.0*0.5},
		{"deepseek", Pricing{0.14, 0.28}, 1000, 1000, 0.42},
		{"exact-1000", Pricing{2.5, 10.0}, 1000, 0, 2.5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.pricing.CalculateCost(tc.inputTokens, tc.outputTokens)
			if math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("CalculateCost(%d, %d) = %v, want %v", tc.inputTokens, tc.outputTokens, got, tc.want)
			}
		})
	}
}
