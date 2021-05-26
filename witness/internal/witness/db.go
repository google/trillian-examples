package witness

import (
	"database/sql"
	"fmt"
)

// Database simply gets and puts things into persistent storage.
type Database struct {
	db *sql.DB
}

// NewDatabase creates a new database, initializing it if needed.
func NewDatabase(db *sql.DB) (*Database, error) {
	d := &Database{
		db : db,
	}
	return d, d.init()
}

func (d *Database) init() error {
	_, err := d.db.Exec("CREATE TABLE IF NOT EXISTS chkpts (key BLOB PRIMARY KEY, size INT, data BLOB)")
	return err
}

// GetLatest reads the latest checkpoint written to the DB for a given log.
func (d *Database) GetLatest(logPK string) (*Chkpt, error) {
	var maxChkpt *Chkpt
	row := d.db.QueryRow("SELECT data FROM chkpts WHERE key = ? ORDER BY size DESC LIMIT 1", logPK)
	if err := row.Err(); err != nil {
		return nil, err
	}
	if err := row.Scan(&maxChkpt); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("Unknown public key: %q", err)
		}
		return nil, err
	}
	return maxChkpt, nil
}

// SetCheckpoint writes the checkpoint to the DB for a given logId.
func (d *Database) SetCheckpoint(logPK string, c Chkpt) error {
	_, err := d.db.Exec("INSERT OR IGNORE INTO chkpts (key, size, data) VALUES (?, ?, ?)", logPK, c.Parsed.Size, c)
	return err
}
