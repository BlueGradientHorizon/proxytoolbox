package worker

import (
	"encoding/binary"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"
)

func pbB(s string) []byte { return []byte(s) }

func pbS(b []byte) string { return string(b) }

func pbStrings(ss []string) [][]byte {
	if len(ss) == 0 {
		return nil
	}
	out := make([][]byte, len(ss))
	for i, s := range ss {
		out[i] = pbB(s)
	}
	return out
}

func pbStringSlice(bb [][]byte) []string {
	if len(bb) == 0 {
		return nil
	}
	out := make([]string, len(bb))
	for i, b := range bb {
		out[i] = pbS(b)
	}
	return out
}

func pbStringMap(m map[string]string) map[string][]byte {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string][]byte, len(m))
	for k, v := range m {
		out[k] = pbB(v)
	}
	return out
}

func pbMapToString(m map[string][]byte) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = pbS(v)
	}
	return out
}

func WritePB(w io.Writer, msg proto.Message) error {
	b, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(b)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

func ReadPB(r io.Reader, msg proto.Message) error {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n == 0 {
		return fmt.Errorf("empty protobuf message")
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	return proto.Unmarshal(buf, msg)
}

func PBBytesToString(b []byte) string { return pbS(b) }

func StringToPBBytes(s string) []byte { return pbB(s) }

func PBTagsToStrings(tags [][]byte) []string { return pbStringSlice(tags) }

func StringsToPBTags(tags []string) [][]byte { return pbStrings(tags) }
