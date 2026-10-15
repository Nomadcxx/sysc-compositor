// Package control is sysc-compositor's session-local control socket: a 0600,
// peer-verified, JSON-lines channel over which one client drives the injected
// seat and any client may set Stop and read facts (session design D3).
//
// It is plain Go and links no cgo. Every effect reaches wlroots through the
// Seat interface, whose compositor implementation posts onto the compositor
// thread.
package control

// Error codes of the control socket's own envelope (design D5). They are not
// sysc-shell's Tier 1 codes and the two never mix.
const (
	CodeStopped       = "stopped-by-user"
	CodeUnknownTarget = "unknown-target"
	CodeUnknownVerb   = "unknown-verb"
	CodeBadRequest    = "bad-request"
	CodeNotAttached   = "not-attached"
	CodeBusy          = "busy"
	CodeUnsupported   = "unsupported"
	CodeFailed        = "failed"
)

// MaxLine bounds one request line, newline excluded.
const MaxLine = 64 << 10

// Request is one line from a client. Fields a verb does not use are ignored.
type Request struct {
	ID      int64   `json:"id"`
	Verb    string  `json:"verb"`
	Name    string  `json:"name,omitempty"`
	Target  string  `json:"target,omitempty"`
	X       float64 `json:"x,omitempty"`
	Y       float64 `json:"y,omitempty"`
	Button  string  `json:"button,omitempty"`
	Key     string  `json:"key,omitempty"`
	Text    string  `json:"text,omitempty"`
	Pressed bool    `json:"pressed,omitempty"`
}

// Reply answers exactly one Request.
type Reply struct {
	ID      int64          `json:"id"`
	OK      bool           `json:"ok,omitempty"`
	Error   string         `json:"error,omitempty"`
	Message string         `json:"message,omitempty"`
	Window  *Window        `json:"window,omitempty"`
	Windows []Window       `json:"windows,omitempty"`
	Facts   map[string]any `json:"facts,omitempty"`
}

// Fact is pushed to subscribers when a published value changes.
type Fact struct {
	Fact  string `json:"fact"`
	Value any    `json:"value"`
}

// Window is one toplevel as a target. ID is stable for the window's lifetime.
type Window struct {
	ID     uint64 `json:"id"`
	AppID  string `json:"app_id"`
	Title  string `json:"title"`
	PID    int    `json:"pid"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

func fail(id int64, code, message string) Reply {
	return Reply{ID: id, Error: code, Message: message}
}
