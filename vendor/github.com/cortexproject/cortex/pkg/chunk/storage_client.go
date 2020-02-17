package chunk

import (
	"context"
	"errors"
	"io"
	"time"
)

// DirDelim is the delimiter used to model a directory structure in an object store.
const DirDelim = "/"

var (
	// ErrStorageObjectNotFound when object storage does not have requested object
	ErrStorageObjectNotFound = errors.New("object not found in storage")
	// ErrMethodNotImplemented when any of the storage clients do not implement a method
	ErrMethodNotImplemented = errors.New("method is not implemented")
)

// IndexClient is a client for the storage of the index (e.g. DynamoDB or Bigtable).
type IndexClient interface {
	Stop()

	// For the write path.
	NewWriteBatch() WriteBatch
	BatchWrite(context.Context, WriteBatch) error

	// For the read path.
	QueryPages(ctx context.Context, queries []IndexQuery, callback func(IndexQuery, ReadBatch) (shouldContinue bool)) error
}

// ObjectClient is for storing and retrieving chunks.
type ObjectClient interface {
	Stop()

	PutChunks(ctx context.Context, chunks []Chunk) error
	GetChunks(ctx context.Context, chunks []Chunk) ([]Chunk, error)
	DeleteChunk(ctx context.Context, chunkID string) error
}

// ObjectAndIndexClient allows optimisations where the same client handles both
type ObjectAndIndexClient interface {
	PutChunkAndIndex(ctx context.Context, c Chunk, index WriteBatch) error
}

// WriteBatch represents a batch of writes.
type WriteBatch interface {
	Add(tableName, hashValue string, rangeValue []byte, value []byte)
	Delete(tableName, hashValue string, rangeValue []byte)
}

// ReadBatch represents the results of a QueryPages.
type ReadBatch interface {
	Iterator() ReadBatchIterator
}

// ReadBatchIterator is an iterator over a ReadBatch.
type ReadBatchIterator interface {
	Next() bool
	RangeValue() []byte
	Value() []byte
}

// StorageObject holds properties of objects in object store
type StorageObject struct {
	Key        string
	ModifiedAt time.Time
}

// ObjectClient is for managing raw objects.
type StorageClient interface {
	GetObject(ctx context.Context, objectKey string) (io.ReadCloser, error)
	PutObject(ctx context.Context, objectKey string, object io.ReadSeeker) error
	DeleteObject(ctx context.Context, objectKey string) error
	List(ctx context.Context, prefix string) ([]StorageObject, error)
}
