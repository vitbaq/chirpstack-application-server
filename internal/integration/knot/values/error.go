package values

const (
	ErrorTimeOut      string = "timeOut"
	UnhandledError    string = "ERROR WITHOUT HANDLER"
	ErrorRegistered   string = "thing is already registered"
	ErrorNoConfig     string = "thing's config not provided"
	ErrorFailMetadata string = "failed to validate if config is valid: error getting thing metadata: thing not found on thing's service"
	NoError           string = ""
)
