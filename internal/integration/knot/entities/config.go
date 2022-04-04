package entities

// Config represents the thing's config
type Config struct {
	SensorID int    `mapstructure: "sensorId" json:"sensorId"`
	Schema   Schema `mapstructure: "schema" json:"schema,omitempty"`
	Event    Event  `mapstructure: "event" json:"event,omitempty"`
}
