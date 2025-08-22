// Package resp implements the Redis Serialization Protocol (RESP) for HyperCache
package resp

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// RESP data types
const (
	TypeSimpleString = '+'
	TypeError        = '-'
	TypeInteger      = ':'
	TypeBulkString   = '$'
	TypeArray        = '*'
)

// Value represents a RESP value of any type
type Value struct {
	Type  byte
	Raw   []byte
	Str   string
	Int   int64
	Array []Value
	Null  bool
}

// Parser handles RESP protocol parsing
type Parser struct {
	reader *bufio.Reader
}

// NewParser creates a new RESP parser
func NewParser(r io.Reader) *Parser {
	return &Parser{
		reader: bufio.NewReader(r),
	}
}

// Parse reads and parses a complete RESP value from the input
func (p *Parser) Parse() (*Value, error) {
	return p.parseValue()
}

// parseValue parses a single RESP value
func (p *Parser) parseValue() (*Value, error) {
	// Read the type indicator
	typeByte, err := p.reader.ReadByte()
	if err != nil {
		return nil, err
	}
	
	switch typeByte {
	case TypeSimpleString:
		return p.parseSimpleString()
	case TypeError:
		return p.parseError()
	case TypeInteger:
		return p.parseInteger()
	case TypeBulkString:
		return p.parseBulkString()
	case TypeArray:
		return p.parseArray()
	default:
		return nil, fmt.Errorf("invalid RESP type: %c", typeByte)
	}
}

// parseSimpleString parses a simple string (+OK\r\n)
func (p *Parser) parseSimpleString() (*Value, error) {
	line, err := p.readLine()
	if err != nil {
		return nil, err
	}
	
	return &Value{
		Type: TypeSimpleString,
		Str:  line,
		Raw:  []byte("+" + line + "\r\n"),
	}, nil
}

// parseError parses an error (-ERR message\r\n)
func (p *Parser) parseError() (*Value, error) {
	line, err := p.readLine()
	if err != nil {
		return nil, err
	}
	
	return &Value{
		Type: TypeError,
		Str:  line,
		Raw:  []byte("-" + line + "\r\n"),
	}, nil
}

// parseInteger parses an integer (:123\r\n)
func (p *Parser) parseInteger() (*Value, error) {
	line, err := p.readLine()
	if err != nil {
		return nil, err
	}
	
	num, err := strconv.ParseInt(line, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid integer: %s", line)
	}
	
	return &Value{
		Type: TypeInteger,
		Int:  num,
		Raw:  []byte(":" + line + "\r\n"),
	}, nil
}

// parseBulkString parses a bulk string ($6\r\nfoobar\r\n or $-1\r\n for null)
func (p *Parser) parseBulkString() (*Value, error) {
	line, err := p.readLine()
	if err != nil {
		return nil, err
	}
	
	length, err := strconv.Atoi(line)
	if err != nil {
		return nil, fmt.Errorf("invalid bulk string length: %s", line)
	}
	
	// Handle null bulk string
	if length == -1 {
		return &Value{
			Type: TypeBulkString,
			Null: true,
			Raw:  []byte("$-1\r\n"),
		}, nil
	}
	
	// Handle empty bulk string
	if length == 0 {
		// Still need to consume the \r\n
		_, err := p.readLine()
		if err != nil {
			return nil, err
		}
		return &Value{
			Type: TypeBulkString,
			Str:  "",
			Raw:  []byte("$0\r\n\r\n"),
		}, nil
	}
	
	// Read the string data
	data := make([]byte, length)
	_, err = io.ReadFull(p.reader, data)
	if err != nil {
		return nil, err
	}
	
	// Read the trailing \r\n
	crlf := make([]byte, 2)
	_, err = io.ReadFull(p.reader, crlf)
	if err != nil {
		return nil, err
	}
	
	if crlf[0] != '\r' || crlf[1] != '\n' {
		return nil, fmt.Errorf("expected CRLF after bulk string")
	}
	
	raw := []byte(fmt.Sprintf("$%d\r\n%s\r\n", length, data))
	
	return &Value{
		Type: TypeBulkString,
		Str:  string(data),
		Raw:  raw,
	}, nil
}

// parseArray parses an array (*2\r\n$3\r\nfoo\r\n$3\r\nbar\r\n)
func (p *Parser) parseArray() (*Value, error) {
	line, err := p.readLine()
	if err != nil {
		return nil, err
	}
	
	length, err := strconv.Atoi(line)
	if err != nil {
		return nil, fmt.Errorf("invalid array length: %s", line)
	}
	
	// Handle null array
	if length == -1 {
		return &Value{
			Type: TypeArray,
			Null: true,
			Raw:  []byte("*-1\r\n"),
		}, nil
	}
	
	// Handle empty array
	if length == 0 {
		return &Value{
			Type:  TypeArray,
			Array: []Value{},
			Raw:   []byte("*0\r\n"),
		}, nil
	}
	
	// Parse array elements
	elements := make([]Value, length)
	var rawBuilder strings.Builder
	rawBuilder.WriteString(fmt.Sprintf("*%d\r\n", length))
	
	for i := 0; i < length; i++ {
		element, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		elements[i] = *element
		rawBuilder.Write(element.Raw)
	}
	
	return &Value{
		Type:  TypeArray,
		Array: elements,
		Raw:   []byte(rawBuilder.String()),
	}, nil
}

// readLine reads a line ending with \r\n and returns the content without CRLF
func (p *Parser) readLine() (string, error) {
	line, err := p.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	
	// Remove \r\n
	if len(line) < 2 || line[len(line)-2:] != "\r\n" {
		return "", fmt.Errorf("line must end with CRLF")
	}
	
	return line[:len(line)-2], nil
}

// Formatter handles RESP response formatting
type Formatter struct{}

// NewFormatter creates a new RESP formatter
func NewFormatter() *Formatter {
	return &Formatter{}
}

// FormatSimpleString formats a simple string response
func (f *Formatter) FormatSimpleString(s string) []byte {
	return []byte(fmt.Sprintf("+%s\r\n", s))
}

// FormatError formats an error response
func (f *Formatter) FormatError(err string) []byte {
	return []byte(fmt.Sprintf("-%s\r\n", err))
}

// FormatInteger formats an integer response
func (f *Formatter) FormatInteger(i int64) []byte {
	return []byte(fmt.Sprintf(":%d\r\n", i))
}

// FormatBulkString formats a bulk string response
func (f *Formatter) FormatBulkString(s string) []byte {
	if s == "" {
		return []byte("$0\r\n\r\n")
	}
	return []byte(fmt.Sprintf("$%d\r\n%s\r\n", len(s), s))
}

// FormatBulkBytes formats bulk bytes response
func (f *Formatter) FormatBulkBytes(data []byte) []byte {
	if len(data) == 0 {
		return []byte("$0\r\n\r\n")
	}
	return []byte(fmt.Sprintf("$%d\r\n%s\r\n", len(data), data))
}

// FormatNull formats a null bulk string response
func (f *Formatter) FormatNull() []byte {
	return []byte("$-1\r\n")
}

// FormatArray formats an array response
func (f *Formatter) FormatArray(elements [][]byte) []byte {
	if len(elements) == 0 {
		return []byte("*0\r\n")
	}
	
	var result strings.Builder
	result.WriteString(fmt.Sprintf("*%d\r\n", len(elements)))
	
	for _, element := range elements {
		result.Write(element)
	}
	
	return []byte(result.String())
}

// Command represents a parsed Redis command
type Command struct {
	Name string
	Args []string
	Raw  []byte
}

// ParseCommand converts a RESP array value to a Command
func ParseCommand(value *Value) (*Command, error) {
	if value.Type != TypeArray {
		return nil, fmt.Errorf("command must be an array")
	}
	
	if value.Null || len(value.Array) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	
	// First element is the command name
	if value.Array[0].Type != TypeBulkString {
		return nil, fmt.Errorf("command name must be a bulk string")
	}
	
	cmdName := strings.ToUpper(value.Array[0].Str)
	args := make([]string, len(value.Array)-1)
	
	// Remaining elements are arguments
	for i, arg := range value.Array[1:] {
		if arg.Type != TypeBulkString {
			return nil, fmt.Errorf("command arguments must be bulk strings")
		}
		args[i] = arg.Str
	}
	
	return &Command{
		Name: cmdName,
		Args: args,
		Raw:  value.Raw,
	}, nil
}

// Utility functions

// IsArray checks if value is an array
func (v *Value) IsArray() bool {
	return v.Type == TypeArray && !v.Null
}

// IsBulkString checks if value is a bulk string
func (v *Value) IsBulkString() bool {
	return v.Type == TypeBulkString && !v.Null
}

// IsInteger checks if value is an integer
func (v *Value) IsInteger() bool {
	return v.Type == TypeInteger
}

// IsSimpleString checks if value is a simple string
func (v *Value) IsSimpleString() bool {
	return v.Type == TypeSimpleString
}

// IsError checks if value is an error
func (v *Value) IsError() bool {
	return v.Type == TypeError
}

// IsNull checks if value is null
func (v *Value) IsNull() bool {
	return v.Null
}

// String returns a string representation of the value for debugging
func (v *Value) String() string {
	switch v.Type {
	case TypeSimpleString:
		return fmt.Sprintf("SimpleString(%q)", v.Str)
	case TypeError:
		return fmt.Sprintf("Error(%q)", v.Str)
	case TypeInteger:
		return fmt.Sprintf("Integer(%d)", v.Int)
	case TypeBulkString:
		if v.Null {
			return "BulkString(null)"
		}
		return fmt.Sprintf("BulkString(%q)", v.Str)
	case TypeArray:
		if v.Null {
			return "Array(null)"
		}
		return fmt.Sprintf("Array(len=%d)", len(v.Array))
	default:
		return fmt.Sprintf("Unknown(%c)", v.Type)
	}
}
