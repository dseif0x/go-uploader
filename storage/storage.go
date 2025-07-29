package storage

import (
	"io"
)

// Backend defines a common interface for saving files.
type Backend interface {
	SaveFile(name string, data io.Reader) error
}
