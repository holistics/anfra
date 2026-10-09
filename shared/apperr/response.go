package apperr

// Envelope is what every transport sends: {"error": {...}}, so a body is
// self-describing wherever it turns up.
type Envelope struct {
	Error Response `json:"error"`
}

// Response is the wire form of an error, identical over every transport but
// for Status.
type Response struct {
	Code  string `json:"code"`
	Scope Scope  `json:"scope"`
	// Status is the HTTP status, set by the HTTP transport, which assigns one
	// per public code; repeated in the body so it survives a log line or a
	// proxy. A transport with no status leaves it out.
	Status    int            `json:"status,omitzero"`
	Context   []ContextEntry `json:"context,omitempty"`
	Message   string         `json:"message"`
	Details   any            `json:"details,omitzero"`
	RequestID string         `json:"request_id"`
}

// ContextEntry is the wire form of a public step's description
// (appstep.Step.Describe): its id, the parameters its template names, and the
// rendered text — a lowercase verb phrase, made to be chained with ": " before
// the message. It is apperr's contract with clients, so it is apperr's type:
// appstep describes a step, and Response decides how that appears.
type ContextEntry struct {
	Step   string         `json:"step"`
	Params map[string]any `json:"params,omitempty"`
	Text   string         `json:"text"`
}

// Response renders e for a client, with no Status: that is the transport's to
// set. An internal code renders as its public code, with its message and
// details carried over. Its context describes e's public steps, then those
// e discloses of its inner error (Translate). The internal server
// error renders only its default message: any other message for it is a
// diagnosis, and goes to the log — unless DisableErrorEncapsulation was called.
func (e *Error) Response(requestID string) Response {
	pub := e.Code.Public()
	msg := e.clientMessage()
	var context []ContextEntry
	for _, step := range e.disclosedSteps() {
		if d, ok := step.Describe(); ok {
			context = append(context, ContextEntry{Step: d.Name, Params: d.Params, Text: d.Text})
		}
	}
	return Response{
		Code:      pub.String(),
		Scope:     pub.Scope(),
		Context:   context,
		Message:   msg,
		Details:   e.Details,
		RequestID: requestID,
	}
}
