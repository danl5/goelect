package model

// Header is a common structure for both requests and responses.
type Header struct {
	// Node field, which represents the information of a node
	Node Node `json:"node"`
}

// Request represents a structure for the requests.
type Request struct {
	Header
	// CommandCode is the command code.
	CommandCode CommandCode `json:"command_code"`
	// Command is the actual request payload.
	Command any `json:"command"`
}

// Response defines a structure for responses.
type Response struct {
	Header
	// CommandResponse holds the actual response data.
	CommandResponse any `json:"command_response"`
	// Error is an error value; if it's nil, the command was successful.
	Error error `json:"error"`
}

// CommandHandler represents a function that handles command requests and returns responses.
type CommandHandler func(request *Request, response *Response) error

// Transport interface definition that a provider needs to implement.
type Transport interface {
	Server
	Client

	// Decode decodes the raw data into the target object
	// Both the request and response both contain fields of the any type, we need to decode it
	Decode(raw any, target any) error
}

// TransportConfig is an interface representing the contract for a configuration object
// that can be validated.
type TransportConfig interface {
	Validate() error
}

// Server interface defines the fundamental behaviors of a server.
type Server interface {
	// Start initiates the server to begin listening on the specified address.
	Start(listenAddress string, handler CommandHandler, config TransportConfig) error
}

// Client interface defines the fundamental behaviors of a client.
type Client interface {
	// InitConnections initializes a set of connections to the given nodes.
	// It returns an error if any connection fails.
	InitConnections(nodes []*Node, config TransportConfig) error

	// SendRequest sends the command request
	SendRequest(nodeId string, request *Request, response *Response) error
}
