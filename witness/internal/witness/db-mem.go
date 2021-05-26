package witness

import (
	"errors"
	"fmt"

	"github.com/google/trillian-examples/formats/log"
)

// Database simply gets and puts things into persistent storage.
type Database struct {
	// For now this is going to be an in-memory map from logIds to lists of
	// checkpoints.
	logMap map[string][]*log.Checkpoint
}

// GetLatest reads the latest checkpoint written to the DB for a given logId.
func (d *Database) GetLatest(logId string) (*log.Checkpoint, error) {
	if val, ok := d.logMap[logId]; ok {
		return val[len(val)-1], nil
	} else {
		return nil, errors.New(fmt.Sprintf("Witness does not support log %s", logId))
	}
}

// SetCheckpoint writes the checkpoint to the DB for a given logId.
func (d *Database) SetCheckpoint(logId string, c *log.Checkpoint) error {
	if val, ok := d.logMap[logId]; ok {
		val = append(val, c)
	} else {
		d.logMap[logId] = []*log.Checkpoint{c}
	}
	return nil
}
