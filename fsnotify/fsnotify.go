package fsnotify

import (
	"bytes"
	"errors"
	"fmt"
)

type Event struct {
	Name string
	Op   Op
}

type Op uint32

const (
	Create Op = 1 << iota
	Write
	Remove
	Rename
	Chmod
)

func (op Op) String() string {
	var buffer bytes.Buffer

	if op&Create == Create {
		buffer.WriteString("|CREATE")
	}

	if op&Write == Write {
		buffer.WriteString("|WRITE")
	}

	if op&Remove == Remove {
		buffer.WriteString("|REMOVE")
	}

	if op&Rename == Rename {
		buffer.WriteString("|RENAME")
	}

	if op&Chmod == Chmod {
		buffer.WriteString("|CHMOD")
	}

	if buffer.Len() == 0 {
		return ""
	}

	return buffer.String()[1:]
}

func (e Event) String() string {
	return fmt.Sprintf("%q: %s", e.Name, e.Op.String())
}

var ErrEventOver = errors.New("fsnotify quee")
