package entities

// Device represents the device domain entity
type Device struct {
	// KNoT Protocol properties
	ID     string   `json:"id"`
	Token  string   `json:"token,omitempty"`
	Name   string   `json:"name,omitempty"`
	Config []Config `json:"config,omitempty"`

	// LoRaWAN properties

	// KNoT Protocol status
	// status string enum(ready ou online, register, auth, config)
}
