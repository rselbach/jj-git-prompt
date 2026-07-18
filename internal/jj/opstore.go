package jj

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"google.golang.org/protobuf/proto"

	pb "github.com/rselbach/jj-git-prompt/internal/jj/pb/simple_op_storepb"
)

// OperationID is a raw (binary) operation id.
type OperationID string

// Hex returns the operation id in hex form (its file name).
func (id OperationID) Hex() string { return hex.EncodeToString([]byte(id)) }

// Operation is a loaded jj operation. Only the fields the prompt needs are
// retained.
type Operation struct {
	ID      OperationID
	ViewID  []byte
	Parents []OperationID
	// EndTimeMillis is the operation end time, used to pick the most recent
	// head when op heads are divergent.
	EndTimeMillis int64
}

// opHeads lists the operation ids in op_heads/heads. File names are hex ids.
func opHeads(repoPath string) ([]OperationID, error) {
	dir := filepath.Join(repoPath, "op_heads", "heads")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read op heads: %w", err)
	}
	var ids []OperationID
	for _, e := range entries {
		raw, err := hex.DecodeString(e.Name())
		if err != nil {
			continue
		}
		ids = append(ids, OperationID(raw))
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no operation heads in %s", dir)
	}
	return ids, nil
}

func readOperation(repoPath string, id OperationID) (*Operation, error) {
	path := filepath.Join(repoPath, "op_store", "operations", id.Hex())
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read operation: %w", err)
	}
	var opProto pb.Operation
	if err := proto.Unmarshal(buf, &opProto); err != nil {
		return nil, fmt.Errorf("decode operation %s: %w", id.Hex(), err)
	}
	op := &Operation{ID: id, ViewID: opProto.ViewId}
	for _, p := range opProto.Parents {
		op.Parents = append(op.Parents, OperationID(p))
	}
	if meta := opProto.Metadata; meta != nil && meta.EndTime != nil {
		op.EndTimeMillis = meta.EndTime.MillisSinceEpoch
	}
	return op, nil
}

func readView(repoPath string, viewID []byte) (*View, error) {
	path := filepath.Join(repoPath, "op_store", "views", hex.EncodeToString(viewID))
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read view: %w", err)
	}
	var viewProto pb.View
	if err := proto.Unmarshal(buf, &viewProto); err != nil {
		return nil, fmt.Errorf("decode view: %w", err)
	}
	return viewFromProto(&viewProto), nil
}

// headOperation resolves the current operation head. jj merges divergent op
// heads by writing a merge operation; as a read-only consumer we instead
// pick the head with the latest end time, which matches the view a
// subsequent jj command would converge toward closely enough for a prompt.
func headOperation(repoPath string) (*Operation, error) {
	ids, err := opHeads(repoPath)
	if err != nil {
		return nil, err
	}
	var head *Operation
	for _, id := range ids {
		op, err := readOperation(repoPath, id)
		if err != nil {
			return nil, err
		}
		if head == nil || op.EndTimeMillis > head.EndTimeMillis {
			head = op
		}
	}
	return head, nil
}
