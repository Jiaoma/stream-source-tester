package config

func (c *Config) ApplyDefaults() {
	if c.Server.BindAddress == "" {
		c.Server.BindAddress = "0.0.0.0"
	}
	if c.Server.RTSPPort == 0 {
		c.Server.RTSPPort = 8554
	}
	if c.Server.RTPPortMin == 0 {
		c.Server.RTPPortMin = 10000
	}
	if c.Server.RTPPortMax == 0 {
		c.Server.RTPPortMax = 10100
	}

	for i := range c.Inputs {
		if c.Inputs[i].Options == nil {
			c.Inputs[i].Options = map[string]string{}
		}
	}
	for i := range c.Outputs {
		if c.Outputs[i].Options == nil {
			c.Outputs[i].Options = map[string]string{}
		}
	}
	for i := range c.Mutations {
		if c.Mutations[i].Options == nil {
			c.Mutations[i].Options = map[string]string{}
		}
	}
}
