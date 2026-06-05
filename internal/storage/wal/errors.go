package wal

import "errors"

var (
	ErrInvalidCRC = errors.New("corrupt wal record: crc32 mismatch")
	ErrTruncated  = errors.New("corrupt wal record: truncated payload")
	ErrEmptyKey   = errors.New("wal record rejected: key must not be empty")
)
