package provider

// Pricing holds per-1000-tokens pricing for a provider/model combination.
type Pricing struct {
	InputPer1K  float64
	OutputPer1K float64
}

// providerPricing maps provider names to their pricing.
var providerPricing = map[string]Pricing{
	"anthropic":    {InputPer1K: 3.0, OutputPer1K: 15.0},
	"openai":       {InputPer1K: 2.5, OutputPer1K: 10.0},
	"deepseek":     {InputPer1K: 0.14, OutputPer1K: 0.28},
	"nvidia":       {InputPer1K: 0.0, OutputPer1K: 0.0},
	"google":       {InputPer1K: 0.0, OutputPer1K: 0.0},
	"groq":         {InputPer1K: 0.1, OutputPer1K: 0.2},
	"cerebras":     {InputPer1K: 0.6, OutputPer1K: 1.2},
	"xai":          {InputPer1K: 1.0, OutputPer1K: 3.0},
	"openrouter":   {InputPer1K: 0.5, OutputPer1K: 1.0},
	"mistral":      {InputPer1K: 0.3, OutputPer1K: 0.9},
	"minimax":      {InputPer1K: 0.1, OutputPer1K: 0.2},
	"minimax-cn":   {InputPer1K: 0.1, OutputPer1K: 0.2},
	"moonshotai":   {InputPer1K: 0.3, OutputPer1K: 0.6},
	"moonshotai-cn":{InputPer1K: 0.3, OutputPer1K: 0.6},
	"huggingface":  {InputPer1K: 0.5, OutputPer1K: 1.0},
	"fireworks":    {InputPer1K: 0.2, OutputPer1K: 0.8},
	"together":     {InputPer1K: 0.3, OutputPer1K: 0.6},
	"novita":       {InputPer1K: 0.1, OutputPer1K: 0.3},
	"xiaomi":       {InputPer1K: 0.05, OutputPer1K: 0.1},
	"qianfan":      {InputPer1K: 0.1, OutputPer1K: 0.2},
	"volcengine":   {InputPer1K: 0.1, OutputPer1K: 0.2},
	"dashscope":    {InputPer1K: 0.1, OutputPer1K: 0.2},
	"hunyuan":      {InputPer1K: 0.1, OutputPer1K: 0.2},
}

// GetPricing returns the pricing for a provider name.
func GetPricing(providerName string) Pricing {
	return providerPricing[providerName]
}

// CalculateCost calculates the estimated cost for given token usage.
func (p Pricing) CalculateCost(inputTokens, outputTokens int) float64 {
	if p.InputPer1K == 0 && p.OutputPer1K == 0 {
		return 0
	}
	inputCost := float64(inputTokens) / 1000.0 * p.InputPer1K
	outputCost := float64(outputTokens) / 1000.0 * p.OutputPer1K
	return inputCost + outputCost
}
