package entities

// Event represents the thing's event
type Event struct {
	Change         bool        `mapstructure:"change" json:"change"`
	TimeSec        int         `mapstructure:"timeSec" json:"timeSec,omitempty"`
	LowerThreshold interface{} `mapstructure:"lowerThreshold" json:"lowerThreshold,omitempty"`
	UpperThreshold interface{} `mapstructure:"upperThreshold" json:"upperThreshold,omitempty"`
}
