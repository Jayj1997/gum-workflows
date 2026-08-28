package node

import "errors"

// ErrorKind identifies whether a node error ends the workflow or may be
// recovered through human interaction.
type ErrorKind string

const (
	// ErrorKindStructural marks an error that makes the workflow unable to continue.
	ErrorKindStructural ErrorKind = "structural"
	// ErrorKindInteraction marks an agent quality error that advice may recover.
	ErrorKindInteraction ErrorKind = "interaction"
)

type classifiedError struct {
	kind ErrorKind
	err  error
}

func (e classifiedError) Error() string { return e.err.Error() }
func (e classifiedError) Unwrap() error { return e.err }
func (e classifiedError) errorKind() ErrorKind {
	return e.kind
}

// Structural marks err as a structural error. A nil error remains nil.
func Structural(err error) error {
	if err == nil {
		return nil
	}
	return classifiedError{kind: ErrorKindStructural, err: err}
}

// Interaction marks err as an interaction error. A nil error remains nil.
func Interaction(err error) error {
	if err == nil {
		return nil
	}
	return classifiedError{kind: ErrorKindInteraction, err: err}
}

// ErrorKindOf returns the nearest explicit classification in err's unwrap
// chain. Unclassified errors default to structural.
func ErrorKindOf(err error) ErrorKind {
	var classified interface{ errorKind() ErrorKind }
	if errors.As(err, &classified) {
		return classified.errorKind()
	}
	return ErrorKindStructural
}
