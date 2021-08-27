package network

import (
	"github.com/brocaar/chirpstack-application-server/internal/integration/knot/entities"
)

// DeviceRegisterRequest represents the outgoing register device request message
type DeviceRegisterRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// DeviceRegisteredResponse represents the incoming register device response message
type DeviceRegisteredResponse struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Token string  `json:"token"`
	Error *string `json:"error"`
}

// DeviceUnregisterRequest represents the outgoing unregister device request message
type DeviceUnregisterRequest struct {
	ID string `json:"id"`
}

// DeviceUnregisteredResponse represents the incoming unregister device response message
type DeviceUnregisteredResponse struct {
	ID    string  `json:"id"`
	Error *string `json:"error"`
}

// ConfigUpdateRequest represents the outgoing update config request message
type ConfigUpdateRequest struct {
	ID     string            `json:"id"`
	Config []entities.Config `json:"config,omitempty"`
}

// ConfigUpdatedResponse represents the incoming update config response message
type ConfigUpdatedResponse struct {
	ID      string            `json:"id"`
	Config  []entities.Config `json:"config,omitempty"`
	Changed bool              `json:"changed"`
	Error   *string           `json:"error"`
}

// DeviceAuthRequest represents the outgoing auth device command request
type DeviceAuthRequest struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}

// DeviceAuthResponse represents the incoming auth device command response
type DeviceAuthResponse struct {
	ID    string  `json:"id"`
	Error *string `json:"error"`
}

// DataSent represents the data sent to the KNoT Cloud
type DataSent struct {
	ID   string          `json:"id"`
	Data []entities.Data `json:"data"`
}
