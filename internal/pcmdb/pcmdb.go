// Package pcmdb reads the database file (OfficialRelease.cdb) of a locally
// installed Pro Cycling Manager mod, such as WorldDB.
//
// Format (reverse-engineered, Nov 2026): a 12-byte header (0xFFFFFFFF, the
// uncompressed size, the compressed size) and a zlib stream. Inside, a tree
// of blocks: 0xAAAAAAAA, total size, tag, a reserved word, a has-name flag
// (then name length, name, padding to 4 bytes), 0xBBBBBBBB, the payload, and
// a 4-byte end marker. Lists start with 0xDDDDDDDD and a count. Tables (tag
// 0x10) store their columns (tag 0x20) one after another: 0x21 holds the
// column type (0 int32, 1 float32, 2 string, 11 int list), 0x22 one value
// per row and, for strings, 0x23 the strings in row order, each row's value
// being its byte length including the terminating zero.
//
// The data belong to the mod's authors: read it from the local install and
// never commit it or bulk data derived from it.
package pcmdb

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
)

const (
	markBlock = 0xAAAAAAAA
	markBody  = 0xBBBBBBBB
	markList  = 0xDDDDDDDD

	tagTable  = 0x10
	tagColumn = 0x20
	tagType   = 0x21
	tagValues = 0x22
	tagPool   = 0x23
)

// Column types.
const (
	TypeInt    = 0
	TypeFloat  = 1
	TypeString = 2
)

// DB is a decoded database: its tables by name.
type DB struct {
	Tables map[string]*Table
}

// Table is a decoded table, column by column.
type Table struct {
	Name    string
	Rows    int
	Columns map[string]*Column
}

// Column is one column: Ints for int columns, Floats for float columns,
// Strings for string columns.
type Column struct {
	Type    int
	Ints    []int32
	Floats  []float32
	Strings []string
}

// Int returns a row's integer value (0 when absent or not an int column).
func (t *Table) Int(col string, row int) int {
	c := t.Columns[col]
	if c == nil || c.Type != TypeInt || row >= len(c.Ints) {
		return 0
	}
	return int(c.Ints[row])
}

// String returns a row's string value ("" when absent).
func (t *Table) String(col string, row int) string {
	c := t.Columns[col]
	if c == nil || c.Type != TypeString || row >= len(c.Strings) {
		return ""
	}
	return c.Strings[row]
}

// Open reads and decodes a database file.
func Open(path string) (*DB, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Decode(raw)
}

// Decode decodes a database file's bytes.
func Decode(raw []byte) (*DB, error) {
	if len(raw) < 12 || binary.LittleEndian.Uint32(raw) != 0xFFFFFFFF {
		return nil, fmt.Errorf("pcmdb: not a database file")
	}
	size := binary.LittleEndian.Uint32(raw[4:])
	zr, err := zlib.NewReader(bytes.NewReader(raw[12:]))
	if err != nil {
		return nil, fmt.Errorf("pcmdb: %w", err)
	}
	b, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("pcmdb: %w", err)
	}
	if uint32(len(b)) != size {
		return nil, fmt.Errorf("pcmdb: size %d, header says %d", len(b), size)
	}
	db := &DB{Tables: map[string]*Table{}}
	start := bytes.Index(b, []byte{0xAA, 0xAA, 0xAA, 0xAA})
	if start < 0 {
		return nil, fmt.Errorf("pcmdb: no blocks")
	}
	nodes, err := parse(b, start, len(b))
	if err != nil {
		return nil, err
	}
	var visit func([]node)
	visit = func(ns []node) {
		for _, n := range ns {
			if n.tag == tagTable {
				if t, err := table(b, n); err == nil {
					db.Tables[t.Name] = t
				}
				continue
			}
			visit(n.kids)
		}
	}
	visit(nodes)
	return db, nil
}

type node struct {
	tag        uint32
	name       string
	body, stop int // payload range
	kids       []node
}

func u32(b []byte, off int) uint32 { return binary.LittleEndian.Uint32(b[off:]) }

// parse reads the sibling blocks in b[off:end].
func parse(b []byte, off, end int) ([]node, error) {
	var out []node
	for off+4 <= end && u32(b, off) == markBlock {
		if off+20 > end {
			return nil, fmt.Errorf("pcmdb: truncated block at %d", off)
		}
		size := int(u32(b, off+4))
		tag := u32(b, off+8)
		has := u32(b, off+16)
		p := off + 20
		name := ""
		if has != 0 {
			n := int(u32(b, p))
			p += 4
			if p+n > end {
				return nil, fmt.Errorf("pcmdb: bad name at %d", off)
			}
			name = string(bytes.TrimRight(b[p:p+n], "\x00"))
			p += n
			p = (p + 3) &^ 3
		}
		if p+4 > end || u32(b, p) != markBody {
			return nil, fmt.Errorf("pcmdb: missing body marker at %d", off)
		}
		p += 4
		stop := off + size - 4
		if size < 24 || stop > end || stop < p {
			return nil, fmt.Errorf("pcmdb: bad block size at %d", off)
		}
		n := node{tag: tag, name: name, body: p, stop: stop}
		q := p
		if q+8 <= stop && u32(b, q) == markList {
			q += 8
		}
		if q+4 <= stop && u32(b, q) == markBlock {
			kids, err := parse(b, q, stop)
			if err != nil {
				return nil, err
			}
			n.kids = kids
		}
		out = append(out, n)
		off += size
	}
	return out, nil
}

// table decodes a table node.
func table(b []byte, tn node) (*Table, error) {
	t := &Table{Name: tn.name, Columns: map[string]*Column{}}
	var cols []node
	var find func([]node)
	find = func(ns []node) {
		for _, n := range ns {
			if n.tag == tagColumn {
				cols = append(cols, n)
				continue
			}
			find(n.kids)
		}
	}
	find(tn.kids)
	for _, cn := range cols {
		parts := map[uint32]node{}
		for _, k := range cn.kids {
			parts[k.tag] = k
		}
		tp, ok1 := parts[tagType]
		vals, ok2 := parts[tagValues]
		if !ok1 || !ok2 || tp.stop-tp.body < 4 {
			continue
		}
		c := &Column{Type: int(u32(b, tp.body))}
		n := (vals.stop - vals.body) / 4
		switch c.Type {
		case TypeFloat:
			c.Floats = make([]float32, n)
			for i := range c.Floats {
				c.Floats[i] = math.Float32frombits(u32(b, vals.body+4*i))
			}
		case TypeInt, TypeString:
			c.Ints = make([]int32, n)
			for i := range c.Ints {
				c.Ints[i] = int32(u32(b, vals.body+4*i))
			}
			if c.Type == TypeString {
				pool, ok := parts[tagPool]
				if !ok {
					continue
				}
				c.Strings = make([]string, n)
				pos := pool.body + 4
				for i, l := range c.Ints {
					if l > 0 && pos+int(l) <= pool.stop {
						c.Strings[i] = string(b[pos : pos+int(l)-1])
						pos += int(l)
					}
				}
				c.Ints = nil
			}
		default:
			continue // int lists etc. aren't needed
		}
		t.Columns[cn.name] = c
		if n > t.Rows {
			t.Rows = n
		}
	}
	return t, nil
}
