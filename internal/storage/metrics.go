package storage

// Metrics provides hooks for tracing internal storage engine events.
type Metrics interface {
	RecordFlush(durationMs, bytesWritten int64)
	RecordCompaction(durationMs, bytesRead, bytesWritten int64)
	RecordWriteStall(durationMs int64)
	RecordReadAmplification(filesProbed int)
}

// nopMetrics is a no-op implementation of Metrics.
type nopMetrics struct{}

func (n nopMetrics) RecordFlush(durationMs, bytesWritten int64)                 {}
func (n nopMetrics) RecordCompaction(durationMs, bytesRead, bytesWritten int64) {}
func (n nopMetrics) RecordWriteStall(durationMs int64)                          {}
func (n nopMetrics) RecordReadAmplification(filesProbed int)                    {}
