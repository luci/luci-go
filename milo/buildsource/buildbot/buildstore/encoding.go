package buildstore

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
)

// encode marshals src to JSON and compresses it.
func encode(src interface{}) ([]byte, error) {
	jsoninsh, err := json.Marshal(src)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	gsw := gzip.NewWriter(&buf)
	_, err = gsw.Write(jsoninsh)
	if err != nil {
		return nil, err
	}
	err = gsw.Close()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// decode decompresses data and unmarshals into dest as JSON.
func decode(dest interface{}, data []byte) error {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	err = json.NewDecoder(reader).Decode(dest)
	if err != nil {
		return err
	}
	return reader.Close()
}
