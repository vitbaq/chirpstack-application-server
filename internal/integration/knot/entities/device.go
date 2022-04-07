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
	KnotWaitReg            = "waitRegisterResponse"
	KnotWaitAuth           = "waitAutheResponse"
	KnotWaitConfig         = "waitConfigResponse"
	KnotOff                = "ignoreDevice"
	KnotAlreadyReg         = "alreadyRegistered"
	KnotWaitUnreg          = "waitUnregister"
)

// Device represents the device domain entity
type Device struct {
	// KNoT Protocol properties
	ID     string   `mapstructure: "id" json:"id,omitempty"`
	Token  string   `mapstructure: "token" json:"token,omitempty"`
	Name   string   `mapstructure: "name" json:"name,omitempty"`
	Config []Config `mapstructure: "config" json:"config,omitempty"`
	State  string   `json:"state,omitempty"`
	Data   []Data   `json:"data,omitempty"`
	Error  string

	// LoRaWAN properties

	// KNoT Protocol status
	// status string enum(ready ou online, register, auth, config)
}
