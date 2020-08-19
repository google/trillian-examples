package audit

import (
	"context"
	"database/sql"
	"fmt"
)

// Metadata is the semantic data that is contained within the leaves of the log.
type Metadata struct {
	module, version, repoHash, modHash string
}

// Database provides read/write access to the local copy of the SumDB.
type Database struct {
	db *sql.DB
}

// NewDatabase creates a Database using the contents of the given filepath.NewDatabase.
// If the file doesn't exist it will be created.
func NewDatabase(location string) (*Database, error) {
	db, err := sql.Open("sqlite3", location)
	if err != nil {
		return nil, err
	}
	return &Database{
		db: db,
	}, nil
}

// Init creates the database tables if needed.
func (d *Database) Init() error {
	if _, err := d.db.Exec("CREATE TABLE IF NOT EXISTS leaves (id INTEGER PRIMARY KEY, data BLOB)"); err != nil {
		return err
	}
	if _, err := d.db.Exec("CREATE TABLE IF NOT EXISTS tiles (height INTEGER, level INTEGER, offset INTEGER, hashes BLOB, PRIMARY KEY (height, level, offset))"); err != nil {
		return err
	}
	_, err := d.db.Exec("CREATE TABLE IF NOT EXISTS leafMetadata (id INTEGER PRIMARY KEY, module TEXT, version TEXT, fileshash TEXT, modhash TEXT)")
	return err
}

// GetHead returns the largest leaf index written.
func (d *Database) GetHead() (int64, error) {
	var head int64
	err := d.db.QueryRow("SELECT MAX(id) AS head FROM leaves").Scan(&head)
	return head, err
}

// WriteLeaves writes the contiguous chunk of leaves, starting at the stated index.
func (d *Database) WriteLeaves(ctx context.Context, start int64, leaves [][]byte) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("BeginTx: %v", err)
	}
	for li, l := range leaves {
		lidx := int64(li) + start
		tx.Exec("INSERT INTO leaves (id, data) VALUES (?, ?)", lidx, l)
	}
	return tx.Commit()
}

// GetLeaves gets a contiguous block of leaves.
func (d *Database) GetLeaves(start int64, count int) ([][]byte, error) {
	var res [][]byte
	rows, err := d.db.Query("SELECT data FROM leaves WHERE id>=? AND id<?", start, start+int64(count))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var data []byte
		rows.Scan(&data)
		res = append(res, data)
	}
	if len(res) != count {
		return nil, fmt.Errorf("failed to read %d leaves, only found %d", count, len(res))
	}
	return res, err
}

// SetLeafMetadata sets the metadata for a contiguous batch of leaves.
func (d *Database) SetLeafMetadata(ctx context.Context, start int64, metadata []Metadata) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("BeginTx: %v", err)
	}
	for mi, m := range metadata {
		midx := int64(mi) + start
		tx.Exec("INSERT INTO leafMetadata (id, module, version, fileshash, modhash) VALUES (?, ?, ?, ?, ?)", midx, m.module, m.version, m.repoHash, m.modHash)
	}
	return tx.Commit()
}

// GetTile gets the leaf hashes for the given tile, or returns an error.
func (d *Database) GetTile(height, level, offset int) ([][]byte, error) {
	var res []byte
	err := d.db.QueryRow("SELECT hashes FROM tiles WHERE height=? AND level=? AND offset=?", height, level, offset).Scan(&res)
	if err != nil {
		return nil, err
	}
	return SplitTile(res, height), nil
}

// SetTile sets the leaf hash data for the given tile.
// The leaf hashes should be 2^height * HashLenBytes long.
func (d *Database) SetTile(height, level, offset int, hashes []byte) error {
	_, err := d.db.Exec("INSERT INTO tiles (height, level, offset, hashes) VALUES (?, ?, ?, ?)", height, level, offset, hashes)
	return err
}

// SplitTile turns the blob that is the leaf hashes in a tile into separate hashes.
func SplitTile(hashes []byte, height int) [][]byte {
	tileWidth := 1 << height
	res := make([][]byte, tileWidth)
	for i := 0; i < tileWidth; i++ {
		hash := hashes[i*HashLenBytes : (i+1)*HashLenBytes]
		res[i] = hash
	}
	return res
}
