package entities

//device state regarding the knot connection
const (
	KnotNew           string = "new"
	KnotRegisterReq          = "registerRequest"
	KnotUnregisterReq        = "registerUnrequest"
	KnotRegistered           = "registered"
	KnotDelete               = "delete"
	KnotForceDelete          = "forceDelete"
	KnotoK                   = "readToSendData"
	KnotAuth                 = "authenticated"
	KnotError                = "error"
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
