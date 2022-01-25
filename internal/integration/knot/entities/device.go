package entities

// States that represent the current status of the device on the Knot network
const (
	KnotNew         string = "new"
	KnotRegistered         = "registered"
	KnotDelete             = "delete"
	KnotForceDelete        = "forceDelete"
	KnotOk                 = "readToSendData"
	KnotAuth               = "authenticated"
	KnotError              = "error"
	KnotWait               = "waitResponse"
)

// Device represents the device domain entity
type Device struct {
	// KNoT Protocol properties
	ID     string   `json:"id"`
	Token  string   `json:"token,omitempty"`
	Name   string   `json:"name,omitempty"`
	Config []Config `json:"config,omitempty"`
	State  string   `json:"state,omitempty"`
	Data   []Data   `json:"data,omitempty"`
	Error  string

	// LoRaWAN properties

	// KNoT Protocol status
	// status string enum(ready ou online, register, auth, config)
}
