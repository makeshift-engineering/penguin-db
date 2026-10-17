package statemachine

import (
	"io"

	consensuspb "github.com/makeshift-engineering/penguin-db/gen/go/consensus/v1"
)

type SnapshotReader interface {
	io.ReadCloser
	Size() int64
}

type StateMachine interface {
	Apply(entry *consensuspb.LogEntry) (any, error)
	Snapshot() (SnapshotReader, error)
	Restore(reader io.Reader) error
	Close() error
}
