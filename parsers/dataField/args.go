package datafield

import (
	"github.com/Reshusk23/sr-me-core/marshal"
)

// ArgsOperationDataFieldParser holds all the components required to create a new instance of data field parser
type ArgsOperationDataFieldParser struct {
	AddressLength int
	Marshalizer   marshal.Marshalizer
}
