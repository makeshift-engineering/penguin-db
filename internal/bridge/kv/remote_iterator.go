package kv

import (
	"context"
	"errors"
	"io"

	storagepb "github.com/makeshift-engineering/penguin-db/gen/go/storage/v1"
)

// Ensure remoteIterator satisfies the Iterator interface at compile time.
var _ Iterator = (*remoteIterator)(nil)

// remoteIterator adapts a gRPC Scan client stream into a kv.Iterator.
type remoteIterator struct {
	streamClient storagepb.StorageService_ScanClient
	cancelFunc   context.CancelFunc
	currentKey   []byte
	currentValue []byte
	isValid      bool
	iterationErr error
}

// newRemoteIterator creates a new remoteIterator and fetches the initial entry.
func newRemoteIterator(streamClient storagepb.StorageService_ScanClient, cancelFunc context.CancelFunc) *remoteIterator {
	iterator := &remoteIterator{
		streamClient: streamClient,
		cancelFunc:   cancelFunc,
	}
	iterator.advance()
	return iterator
}

// Valid reports whether the iterator is currently positioned on a valid entry.
func (it *remoteIterator) Valid() bool {
	return it.isValid && it.iterationErr == nil
}

// Next yields the current key-value pair and advances the iterator to the next entry.
func (it *remoteIterator) Next() (key, value []byte) {
	if !it.isValid {
		return nil, nil
	}
	keyCopy := it.currentKey
	valueCopy := it.currentValue
	it.advance()
	return keyCopy, valueCopy
}

// Close releases resources associated with the iterator stream.
func (it *remoteIterator) Close() {
	if it.cancelFunc != nil {
		it.cancelFunc()
		it.cancelFunc = nil
	}
	it.isValid = false
	it.currentKey = nil
	it.currentValue = nil
}

// Err returns any error encountered during stream iteration.
func (it *remoteIterator) Err() error {
	return it.iterationErr
}

// advance reads the next message from the gRPC scan stream.
func (it *remoteIterator) advance() {
	response, err := it.streamClient.Recv()
	if errors.Is(err, io.EOF) {
		it.isValid = false
		return
	}
	if err != nil {
		it.iterationErr = mapGRPCError(err)
		it.isValid = false
		return
	}

	it.currentKey = response.Key
	it.currentValue = response.Value
	it.isValid = true
}
