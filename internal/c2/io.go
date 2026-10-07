package c2

import (
	"encoding/base64"
	"os"
)

// These thin wrappers exist to:
//   - make the logic in manager.go / handler more readable and avoid repeatedly importing os;
//   - allow future unit testing by replacing with interface abstractions (e.g. internal/storage).

func osMkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func osWriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

func base64Decode(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}
