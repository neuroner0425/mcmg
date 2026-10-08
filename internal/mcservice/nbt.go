package mcservice

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// NBT Tag Type Constants
const (
	TagEnd       byte = 0
	TagByte      byte = 1
	TagShort     byte = 2
	TagInt       byte = 3
	TagLong      byte = 4
	TagFloat     byte = 5
	TagDouble    byte = 6
	TagByteArray byte = 7
	TagString    byte = 8
	TagList      byte = 9
	TagCompound  byte = 10
	TagIntArray  byte = 11
	TagLongArray byte = 12
)

// NBTTag represents a node in the NBT document tree.
type NBTTag struct {
	Type      byte
	Name      string
	Value     any
	Children  []*NBTTag
	ListType  byte
	ListItems []*NBTTag
}

// FindChild searches for a direct child with the given name inside a Compound tag.
func (t *NBTTag) FindChild(name string) *NBTTag {
	if t.Type != TagCompound {
		return nil
	}
	for _, c := range t.Children {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// SetChild sets or replaces a child tag inside a Compound tag.
func (t *NBTTag) SetChild(child *NBTTag) {
	if t.Type != TagCompound {
		return
	}
	for i, c := range t.Children {
		if c.Name == child.Name {
			t.Children[i] = child
			return
		}
	}
	t.Children = append(t.Children, child)
}

// ReadNBT decodes a gzipped NBT byte stream into an NBTTag tree.
func ReadNBT(r io.Reader) (*NBTTag, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("gzip reader error: %w", err)
	}
	defer gz.Close()

	tagType := make([]byte, 1)
	if _, err := io.ReadFull(gz, tagType); err != nil {
		return nil, err
	}
	if tagType[0] == TagEnd {
		return &NBTTag{Type: TagEnd}, nil
	}

	name, err := readNBTString(gz)
	if err != nil {
		return nil, err
	}

	tag := &NBTTag{Type: tagType[0], Name: name}
	if err := readNBTValue(gz, tag); err != nil {
		return nil, err
	}
	return tag, nil
}

func readNBTString(r io.Reader) (string, error) {
	var length uint16
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return "", err
	}
	if length == 0 {
		return "", nil
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func readNBTValue(r io.Reader, tag *NBTTag) error {
	switch tag.Type {
	case TagByte:
		var v int8
		if err := binary.Read(r, binary.BigEndian, &v); err != nil {
			return err
		}
		tag.Value = v
	case TagShort:
		var v int16
		if err := binary.Read(r, binary.BigEndian, &v); err != nil {
			return err
		}
		tag.Value = v
	case TagInt:
		var v int32
		if err := binary.Read(r, binary.BigEndian, &v); err != nil {
			return err
		}
		tag.Value = v
	case TagLong:
		var v int64
		if err := binary.Read(r, binary.BigEndian, &v); err != nil {
			return err
		}
		tag.Value = v
	case TagFloat:
		var v float32
		if err := binary.Read(r, binary.BigEndian, &v); err != nil {
			return err
		}
		tag.Value = v
	case TagDouble:
		var v float64
		if err := binary.Read(r, binary.BigEndian, &v); err != nil {
			return err
		}
		tag.Value = v
	case TagByteArray:
		var length int32
		if err := binary.Read(r, binary.BigEndian, &length); err != nil {
			return err
		}
		buf := make([]byte, length)
		if _, err := io.ReadFull(r, buf); err != nil {
			return err
		}
		tag.Value = buf
	case TagString:
		s, err := readNBTString(r)
		if err != nil {
			return err
		}
		tag.Value = s
	case TagList:
		var elemType byte
		var length int32
		if err := binary.Read(r, binary.BigEndian, &elemType); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &length); err != nil {
			return err
		}
		tag.ListType = elemType
		tag.ListItems = make([]*NBTTag, 0, length)
		for i := int32(0); i < length; i++ {
			item := &NBTTag{Type: elemType}
			if err := readNBTValue(r, item); err != nil {
				return err
			}
			tag.ListItems = append(tag.ListItems, item)
		}
	case TagCompound:
		tag.Children = make([]*NBTTag, 0)
		for {
			var childType byte
			if err := binary.Read(r, binary.BigEndian, &childType); err != nil {
				return err
			}
			if childType == TagEnd {
				break
			}
			childName, err := readNBTString(r)
			if err != nil {
				return err
			}
			child := &NBTTag{Type: childType, Name: childName}
			if err := readNBTValue(r, child); err != nil {
				return err
			}
			tag.Children = append(tag.Children, child)
		}
	case TagIntArray:
		var length int32
		if err := binary.Read(r, binary.BigEndian, &length); err != nil {
			return err
		}
		arr := make([]int32, length)
		if err := binary.Read(r, binary.BigEndian, arr); err != nil {
			return err
		}
		tag.Value = arr
	case TagLongArray:
		var length int32
		if err := binary.Read(r, binary.BigEndian, &length); err != nil {
			return err
		}
		arr := make([]int64, length)
		if err := binary.Read(r, binary.BigEndian, arr); err != nil {
			return err
		}
		tag.Value = arr
	}
	return nil
}

// WriteNBT encodes an NBT document tree into gzipped bytes.
func WriteNBT(w io.Writer, root *NBTTag) error {
	gz := gzip.NewWriter(w)
	defer gz.Close()

	if _, err := gz.Write([]byte{root.Type}); err != nil {
		return err
	}
	if root.Type == TagEnd {
		return nil
	}

	if err := writeNBTString(gz, root.Name); err != nil {
		return err
	}

	if err := writeNBTValue(gz, root); err != nil {
		return err
	}

	return nil
}

func writeNBTString(w io.Writer, s string) error {
	length := uint16(len(s))
	if err := binary.Write(w, binary.BigEndian, length); err != nil {
		return err
	}
	if length > 0 {
		if _, err := io.WriteString(w, s); err != nil {
			return err
		}
	}
	return nil
}

func writeNBTValue(w io.Writer, tag *NBTTag) error {
	switch tag.Type {
	case TagByte:
		return binary.Write(w, binary.BigEndian, tag.Value.(int8))
	case TagShort:
		return binary.Write(w, binary.BigEndian, tag.Value.(int16))
	case TagInt:
		return binary.Write(w, binary.BigEndian, tag.Value.(int32))
	case TagLong:
		return binary.Write(w, binary.BigEndian, tag.Value.(int64))
	case TagFloat:
		return binary.Write(w, binary.BigEndian, tag.Value.(float32))
	case TagDouble:
		return binary.Write(w, binary.BigEndian, tag.Value.(float64))
	case TagByteArray:
		buf := tag.Value.([]byte)
		length := int32(len(buf))
		if err := binary.Write(w, binary.BigEndian, length); err != nil {
			return err
		}
		_, err := w.Write(buf)
		return err
	case TagString:
		return writeNBTString(w, tag.Value.(string))
	case TagList:
		if err := binary.Write(w, binary.BigEndian, tag.ListType); err != nil {
			return err
		}
		length := int32(len(tag.ListItems))
		if err := binary.Write(w, binary.BigEndian, length); err != nil {
			return err
		}
		for _, item := range tag.ListItems {
			if err := writeNBTValue(w, item); err != nil {
				return err
			}
		}
	case TagCompound:
		for _, child := range tag.Children {
			if err := binary.Write(w, binary.BigEndian, child.Type); err != nil {
				return err
			}
			if err := writeNBTString(w, child.Name); err != nil {
				return err
			}
			if err := writeNBTValue(w, child); err != nil {
				return err
			}
		}
		return binary.Write(w, binary.BigEndian, TagEnd)
	case TagIntArray:
		arr := tag.Value.([]int32)
		length := int32(len(arr))
		if err := binary.Write(w, binary.BigEndian, length); err != nil {
			return err
		}
		return binary.Write(w, binary.BigEndian, arr)
	case TagLongArray:
		arr := tag.Value.([]int64)
		length := int32(len(arr))
		if err := binary.Write(w, binary.BigEndian, length); err != nil {
			return err
		}
		return binary.Write(w, binary.BigEndian, arr)
	}
	return nil
}

// ReadLevelDat reads and parses a level.dat file.
func ReadLevelDat(path string) (*NBTTag, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadNBT(f)
}

// WriteLevelDat safely writes an NBT document tree to a level.dat file with backup.
func WriteLevelDat(path string, root *NBTTag) error {
	// Create backup
	if data, err := os.ReadFile(path); err == nil {
		_ = os.WriteFile(path+".bak", data, 0644)
	}

	var buf bytes.Buffer
	if err := WriteNBT(&buf, root); err != nil {
		return err
	}

	return os.WriteFile(path, buf.Bytes(), 0644)
}
