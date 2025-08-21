package modelrepo

type chatOption struct {
	apply func(*chatConfig)
}

func (c *chatOption) SetTemperature(temp float64) {
	c.apply(&chatConfig{temperature: temp})
}

func (c *chatOption) SetMaxTokens(tokens int) {
	c.apply(&chatConfig{maxTokens: tokens})
}

// Internal config to hold settings
type chatConfig struct {
	temperature float64
	maxTokens   int
}

// Functional option constructors
func WithTemperature(temp float64) ChatOption {
	return &chatOption{
		apply: func(config *chatConfig) {
			config.temperature = temp
		},
	}
}

func WithMaxTokens(tokens int) ChatOption {
	return &chatOption{
		apply: func(config *chatConfig) {
			config.maxTokens = tokens
		},
	}
}
