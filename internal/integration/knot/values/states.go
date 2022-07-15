package values

// States that represent the current status of the device on the Knot network
const (
	KNoTNew         string = "new"
	KNoTRegistered  string = "registered"
	KNoTDelete      string = "delete"
	KNoTForceDelete string = "forceDelete"
	KNoTOk          string = "readToSendData"
	KNoTAuth        string = "authenticated"
	KNoTError       string = "error"
	KNoTWaitReg     string = "waitRegisterResponse"
	KNoTWaitAuth    string = "waitAutheResponse"
	KNoTWaitConfig  string = "waitConfigResponse"
	KNoTOff         string = "ignoreDevice"
	KNoTAlreadyReg  string = "alreadyRegistered"
	KNoTWaitUnreg   string = "waitUnregister"
)
