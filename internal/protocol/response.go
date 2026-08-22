package protocol

import (
	"fmt"
	"io"
	"strconv"
)

// Writer writes RESP2 responses.
type Writer struct {
	w io.Writer
}

// NewWriter creates a new RESP writer.
func NewWriter(w io.Writer) *Writer { return &Writer{w: w} }

// WriteSimpleString writes +OK\r\n style response.
func (w *Writer) WriteSimpleString(s string) error {
	_, err := fmt.Fprintf(w.w, "+%s\r\n", s)
	return err
}

// WriteError writes -ERR ...\r\n response.
func (w *Writer) WriteError(msg string) error {
	_, err := fmt.Fprintf(w.w, "-ERR %s\r\n", msg)
	return err
}

// WriteErrorRaw writes -<msg>\r\n without prepending ERR.
func (w *Writer) WriteErrorRaw(msg string) error {
	_, err := fmt.Fprintf(w.w, "-%s\r\n", msg)
	return err
}

// WriteRaw writes raw bytes to the underlying writer.
func (w *Writer) WriteRaw(b []byte) error {
	_, err := w.w.Write(b)
	return err
}

// WriteInteger writes :<n>\r\n response.
func (w *Writer) WriteInteger(n int64) error {
	_, err := fmt.Fprintf(w.w, ":%d\r\n", n)
	return err
}

// WriteBulkString writes $<len>\r\n<data>\r\n response. Pass nil for nil bulk.
func (w *Writer) WriteBulkString(b []byte) error {
	if b == nil {
		_, err := fmt.Fprint(w.w, "$-1\r\n")
		return err
	}
	if _, err := fmt.Fprintf(w.w, "$%d\r\n", len(b)); err != nil {
		return err
	}
	if _, err := w.w.Write(b); err != nil {
		return err
	}
	_, err := fmt.Fprint(w.w, "\r\n")
	return err
}

// WriteBulkStringStr writes a string as bulk.
func (w *Writer) WriteBulkStringStr(s string) error {
	return w.WriteBulkString([]byte(s))
}

// WriteArray writes *<count>\r\n and the array prefix.
func (w *Writer) WriteArray(count int) error {
	if count < 0 {
		_, err := fmt.Fprint(w.w, "*-1\r\n")
		return err
	}
	_, err := fmt.Fprintf(w.w, "*%d\r\n", count)
	return err
}

// WriteNull writes a RESP null bulk string.
func (w *Writer) WriteNull() error {
	_, err := fmt.Fprint(w.w, "$-1\r\n")
	return err
}

// WriteOK writes +OK\r\n.
func (w *Writer) WriteOK() error {
	return w.WriteSimpleString("OK")
}

// WriteStrings writes an array of strings as bulk strings.
func (w *Writer) WriteStrings(strs []string) error {
	if err := w.WriteArray(len(strs)); err != nil {
		return err
	}
	for _, s := range strs {
		if err := w.WriteBulkStringStr(s); err != nil {
			return err
		}
	}
	return nil
}

// WriteBytes writes an array of byte slices.
func (w *Writer) WriteBytes(items [][]byte) error {
	if err := w.WriteArray(len(items)); err != nil {
		return err
	}
	for _, item := range items {
		if err := w.WriteBulkString(item); err != nil {
			return err
		}
	}
	return nil
}

// WriteFloat writes a float64 as a bulk string (Redis convention).
func (w *Writer) WriteFloat(f float64) error {
	return w.WriteBulkStringStr(strconv.FormatFloat(f, 'f', -1, 64))
}
