package migrate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	dg "github.com/dolan-in/dgman/v2"
)

// stepChecksum hashes a step's identity (name + ordinal) and its schema portion
// only — never the Up/Down closures. Alter strings are hashed raw; Ensure types
// are hashed via their canonical dgman schema string.
func stepChecksum(index int, s Step) string {
	h := sha256.New()
	fmt.Fprintf(h, "name:%s\nindex:%d\n", s.Name, index)
	switch {
	case s.Schema.Alter != "":
		h.Write([]byte("alter:" + s.Schema.Alter))
	case len(s.Schema.Ensure) > 0:
		ts := dg.NewTypeSchema()
		ts.Marshal("", s.Schema.Ensure...)
		h.Write([]byte("ensure:" + ts.String()))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// migrationChecksum hashes the ordered concatenation of each step's checksum,
// so editing, adding, removing, or reordering steps changes it.
func migrationChecksum(m Migration) string {
	h := sha256.New()
	for i, s := range m.Steps {
		h.Write([]byte(stepChecksum(i, s)))
	}
	return hex.EncodeToString(h.Sum(nil))
}
