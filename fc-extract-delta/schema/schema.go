package schema

import (
    "encoding/hex"
	"encoding/json"
)

// Class represents the type of test this instance is.
type Class string

var (
	// ClassMessage tests the VM transition over a single message
	ClassMessages Class = "messages"
	// ClassBlock tests the VM transition over a block of messages
	ClassBlock Class = "block"
	// ClassTipset tests the VM transition on a tipset update
	ClassTipset Class = "tipset"
	// ClassChain tests the VM transition across a chain segment
	ClassChain Class = "chain"
)

// Selector provides a filter to indicate what implementations this test is relevant for
type Selector string

// Metadata provides information on the generation of this test case
type Metadata struct {
	ID string
	Version string
	Gen GenerationData
}

// GenerationData tags the source of this test case
type GenerationData struct {
	Source string
	Version string
}

// Preconditions contain a representation of VM state at the beginning of the test
type Preconditions struct {
	StateTree interface{}
}

// Postconditions contain a representation of VM state at th end of the test
type Postconditions struct {
	StateTree interface{}
}

// Message contains a serialization of a VM messate
type Message []byte

// MarshalJSON implements json.Marshal for Message
func (m Message) MarshalJSON() ([]byte, error) {
    return json.Marshal(hex.EncodeToString(m))
}

// TestVector is a single test case
type TestVector struct {
	Class
	Selector
	*Metadata
	*Preconditions
	ApplyMessages []Message
	*Postconditions
}
