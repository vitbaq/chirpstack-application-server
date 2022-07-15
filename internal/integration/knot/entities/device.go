package entities

// Device represents the device domain entity
type Device struct {
	// KNoT Protocol properties
	ID     string   `mapstructure:"id" json:"id,omitempty"`
	Token  string   `mapstructure:"token" json:"token,omitempty"`
	DevEUI string   `mapstructure:"deveui" json:"deveui,omitempty"`
	Name   string   `mapstructure:"name" json:"name,omitempty"`
	Config []Config `mapstructure:"config" json:"config,omitempty"`
	State  string   `json:"state,omitempty"`
	Data   []Data   `json:"data,omitempty"`
	Error  string

	// LoRaWAN properties

	// KNoT Protocol status
	// status string enum(ready ou online, register, auth, config)
}
