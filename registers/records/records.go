package records

// Key types
import "crypto/sha256"

const (
	KTRecord = "record:"
	KTKey    = "key:"
)

func hash(kt string, key string) []byte {
	hash := sha256.Sum256([]byte(kt + key))
	return hash[:]
}

func RecordHash(key string) []byte {
	return hash(KTRecord, key)
}

func KeyHash(index int) []byte {
	return hash(KTKey, string(index))
}
