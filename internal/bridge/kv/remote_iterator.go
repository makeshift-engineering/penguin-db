package kv

import (
	"context"
	"errors"
	"io"

	storagepb "github.com/makeshift-engineering/penguin-db/gen/go/storage/v1"
)

var _ Iterator = (*remoteIterator)(nil)

type remoteIterator struct {
	streamClient storagepb.StorageService_ScanClient
	cancelFunc   context.CancelFunc
	currentKey   []byte
	currentValue []byte
	isValid      bool
	iterationErr error
}

func newRemoteIterator(streamClient storagepb.StorageService_ScanClient, cancelFunc context.CancelFunc) *remoteIterator {
	iterator := &remoteIterator{
		streamClient: streamClient,
		cancelFunc:   cancelFunc,
	}
	iterator.advance()
	return iterator
}

func (it *remoteIterator) Valid() bool {
	return it.isValid && it.iterationErr == nil
}

func (it *remoteIterator) Next() (key, value []byte) {
	if !it.isValid {
		return nil, nil
	}
	keyCopy := it.currentKey
	valueCopy := it.currentValue
	it.advance()
	return keyCopy, valueCopy
}

func (it *remoteIterator) Close() {
	if it.cancelFunc != nil {
		it.cancelFunc()
		it.cancelFunc = nil
	}
	it.isValid = false
	it.currentKey = nil
	it.currentValue = nil
}

func (it *remoteIterator) Err() error {
	return it.iterationErr
}

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
