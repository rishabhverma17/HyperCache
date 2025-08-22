package resp

import (
	"strings"
	"testing"
)

func TestParser_SimpleString(t *testing.T) {
	input := "+OK\r\n"
	parser := NewParser(strings.NewReader(input))
	
	value, err := parser.Parse()
	if err != nil {
		t.Fatalf("Failed to parse simple string: %v", err)
	}
	
	if value.Type != TypeSimpleString {
		t.Errorf("Expected type %c, got %c", TypeSimpleString, value.Type)
	}
	
	if value.Str != "OK" {
		t.Errorf("Expected 'OK', got %q", value.Str)
	}
	
	if string(value.Raw) != input {
		t.Errorf("Expected raw %q, got %q", input, string(value.Raw))
	}
}

func TestParser_Error(t *testing.T) {
	input := "-ERR unknown command\r\n"
	parser := NewParser(strings.NewReader(input))
	
	value, err := parser.Parse()
	if err != nil {
		t.Fatalf("Failed to parse error: %v", err)
	}
	
	if value.Type != TypeError {
		t.Errorf("Expected type %c, got %c", TypeError, value.Type)
	}
	
	if value.Str != "ERR unknown command" {
		t.Errorf("Expected 'ERR unknown command', got %q", value.Str)
	}
}

func TestParser_Integer(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{":0\r\n", 0},
		{":123\r\n", 123},
		{":-456\r\n", -456},
		{":1000000\r\n", 1000000},
	}
	
	for _, test := range tests {
		parser := NewParser(strings.NewReader(test.input))
		value, err := parser.Parse()
		if err != nil {
			t.Fatalf("Failed to parse integer %q: %v", test.input, err)
		}
		
		if value.Type != TypeInteger {
			t.Errorf("Expected type %c, got %c", TypeInteger, value.Type)
		}
		
		if value.Int != test.expected {
			t.Errorf("Expected %d, got %d", test.expected, value.Int)
		}
	}
}

func TestParser_BulkString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		isNull   bool
	}{
		{
			name:     "normal string",
			input:    "$6\r\nfoobar\r\n",
			expected: "foobar",
			isNull:   false,
		},
		{
			name:     "empty string",
			input:    "$0\r\n\r\n",
			expected: "",
			isNull:   false,
		},
		{
			name:     "null string",
			input:    "$-1\r\n",
			expected: "",
			isNull:   true,
		},
		{
			name:     "string with spaces",
			input:    "$11\r\nhello world\r\n",
			expected: "hello world",
			isNull:   false,
		},
		{
			name:     "string with special chars",
			input:    "$10\r\nhello\r\nhi!\r\n",
			expected: "hello\r\nhi!",
			isNull:   false,
		},
	}
	
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parser := NewParser(strings.NewReader(test.input))
			value, err := parser.Parse()
			if err != nil {
				t.Fatalf("Failed to parse bulk string: %v", err)
			}
			
			if value.Type != TypeBulkString {
				t.Errorf("Expected type %c, got %c", TypeBulkString, value.Type)
			}
			
			if value.Null != test.isNull {
				t.Errorf("Expected null=%t, got null=%t", test.isNull, value.Null)
			}
			
			if !test.isNull && value.Str != test.expected {
				t.Errorf("Expected %q, got %q", test.expected, value.Str)
			}
		})
	}
}

func TestParser_Array(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedLength int
		isNull         bool
	}{
		{
			name:           "empty array",
			input:          "*0\r\n",
			expectedLength: 0,
			isNull:         false,
		},
		{
			name:           "null array",
			input:          "*-1\r\n",
			expectedLength: 0,
			isNull:         true,
		},
		{
			name:           "array with two bulk strings",
			input:          "*2\r\n$3\r\nfoo\r\n$3\r\nbar\r\n",
			expectedLength: 2,
			isNull:         false,
		},
		{
			name:           "mixed array",
			input:          "*3\r\n:1\r\n$4\r\ntest\r\n+OK\r\n",
			expectedLength: 3,
			isNull:         false,
		},
	}
	
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parser := NewParser(strings.NewReader(test.input))
			value, err := parser.Parse()
			if err != nil {
				t.Fatalf("Failed to parse array: %v", err)
			}
			
			if value.Type != TypeArray {
				t.Errorf("Expected type %c, got %c", TypeArray, value.Type)
			}
			
			if value.Null != test.isNull {
				t.Errorf("Expected null=%t, got null=%t", test.isNull, value.Null)
			}
			
			if !test.isNull && len(value.Array) != test.expectedLength {
				t.Errorf("Expected length %d, got %d", test.expectedLength, len(value.Array))
			}
		})
	}
}

func TestParser_Command(t *testing.T) {
	// Test Redis command parsing
	input := "*2\r\n$3\r\nGET\r\n$4\r\nkey1\r\n"
	parser := NewParser(strings.NewReader(input))
	
	value, err := parser.Parse()
	if err != nil {
		t.Fatalf("Failed to parse command: %v", err)
	}
	
	cmd, err := ParseCommand(value)
	if err != nil {
		t.Fatalf("Failed to parse command from value: %v", err)
	}
	
	if cmd.Name != "GET" {
		t.Errorf("Expected command 'GET', got %q", cmd.Name)
	}
	
	if len(cmd.Args) != 1 {
		t.Errorf("Expected 1 argument, got %d", len(cmd.Args))
	}
	
	if cmd.Args[0] != "key1" {
		t.Errorf("Expected argument 'key1', got %q", cmd.Args[0])
	}
}

func TestParser_ComplexCommand(t *testing.T) {
	// Test SET key value EX ttl
	input := "*5\r\n$3\r\nSET\r\n$4\r\nkey1\r\n$5\r\nvalue\r\n$2\r\nEX\r\n$2\r\n60\r\n"
	parser := NewParser(strings.NewReader(input))
	
	value, err := parser.Parse()
	if err != nil {
		t.Fatalf("Failed to parse complex command: %v", err)
	}
	
	cmd, err := ParseCommand(value)
	if err != nil {
		t.Fatalf("Failed to parse command from value: %v", err)
	}
	
	expectedArgs := []string{"key1", "value", "EX", "60"}
	if cmd.Name != "SET" {
		t.Errorf("Expected command 'SET', got %q", cmd.Name)
	}
	
	if len(cmd.Args) != len(expectedArgs) {
		t.Errorf("Expected %d arguments, got %d", len(expectedArgs), len(cmd.Args))
	}
	
	for i, expected := range expectedArgs {
		if cmd.Args[i] != expected {
			t.Errorf("Expected arg[%d] = %q, got %q", i, expected, cmd.Args[i])
		}
	}
}

func TestFormatter_SimpleString(t *testing.T) {
	formatter := NewFormatter()
	
	result := formatter.FormatSimpleString("OK")
	expected := "+OK\r\n"
	
	if string(result) != expected {
		t.Errorf("Expected %q, got %q", expected, string(result))
	}
}

func TestFormatter_Error(t *testing.T) {
	formatter := NewFormatter()
	
	result := formatter.FormatError("ERR unknown command")
	expected := "-ERR unknown command\r\n"
	
	if string(result) != expected {
		t.Errorf("Expected %q, got %q", expected, string(result))
	}
}

func TestFormatter_Integer(t *testing.T) {
	formatter := NewFormatter()
	
	tests := []struct {
		input    int64
		expected string
	}{
		{0, ":0\r\n"},
		{123, ":123\r\n"},
		{-456, ":-456\r\n"},
		{1000000, ":1000000\r\n"},
	}
	
	for _, test := range tests {
		result := formatter.FormatInteger(test.input)
		if string(result) != test.expected {
			t.Errorf("Expected %q, got %q", test.expected, string(result))
		}
	}
}

func TestFormatter_BulkString(t *testing.T) {
	formatter := NewFormatter()
	
	tests := []struct {
		input    string
		expected string
	}{
		{"foobar", "$6\r\nfoobar\r\n"},
		{"", "$0\r\n\r\n"},
		{"hello world", "$11\r\nhello world\r\n"},
	}
	
	for _, test := range tests {
		result := formatter.FormatBulkString(test.input)
		if string(result) != test.expected {
			t.Errorf("Expected %q, got %q", test.expected, string(result))
		}
	}
}

func TestFormatter_BulkBytes(t *testing.T) {
	formatter := NewFormatter()
	
	data := []byte{0x01, 0x02, 0x03, 0xFF}
	result := formatter.FormatBulkBytes(data)
	expected := "$4\r\n\x01\x02\x03\xFF\r\n"
	
	if string(result) != expected {
		t.Errorf("Expected %q, got %q", expected, string(result))
	}
}

func TestFormatter_Null(t *testing.T) {
	formatter := NewFormatter()
	
	result := formatter.FormatNull()
	expected := "$-1\r\n"
	
	if string(result) != expected {
		t.Errorf("Expected %q, got %q", expected, string(result))
	}
}

func TestFormatter_Array(t *testing.T) {
	formatter := NewFormatter()
	
	// Test empty array
	result := formatter.FormatArray([][]byte{})
	expected := "*0\r\n"
	
	if string(result) != expected {
		t.Errorf("Expected %q, got %q", expected, string(result))
	}
	
	// Test array with elements
	elements := [][]byte{
		formatter.FormatBulkString("foo"),
		formatter.FormatBulkString("bar"),
	}
	
	result = formatter.FormatArray(elements)
	expected = "*2\r\n$3\r\nfoo\r\n$3\r\nbar\r\n"
	
	if string(result) != expected {
		t.Errorf("Expected %q, got %q", expected, string(result))
	}
}

func TestValue_TypeCheckers(t *testing.T) {
	tests := []struct {
		value    Value
		isArray  bool
		isBulk   bool
		isInt    bool
		isSimple bool
		isError  bool
		isNull   bool
	}{
		{
			value:    Value{Type: TypeArray, Array: []Value{}},
			isArray:  true,
			isBulk:   false,
			isInt:    false,
			isSimple: false,
			isError:  false,
			isNull:   false,
		},
		{
			value:   Value{Type: TypeBulkString, Str: "test"},
			isArray: false, isBulk: true, isInt: false, isSimple: false, isError: false, isNull: false,
		},
		{
			value:   Value{Type: TypeBulkString, Null: true},
			isArray: false, isBulk: false, isInt: false, isSimple: false, isError: false, isNull: true,
		},
		{
			value:   Value{Type: TypeInteger, Int: 123},
			isArray: false, isBulk: false, isInt: true, isSimple: false, isError: false, isNull: false,
		},
		{
			value:   Value{Type: TypeSimpleString, Str: "OK"},
			isArray: false, isBulk: false, isInt: false, isSimple: true, isError: false, isNull: false,
		},
		{
			value:   Value{Type: TypeError, Str: "ERR"},
			isArray: false, isBulk: false, isInt: false, isSimple: false, isError: true, isNull: false,
		},
	}
	
	for i, test := range tests {
		if test.value.IsArray() != test.isArray {
			t.Errorf("Test %d: IsArray() = %t, expected %t", i, test.value.IsArray(), test.isArray)
		}
		if test.value.IsBulkString() != test.isBulk {
			t.Errorf("Test %d: IsBulkString() = %t, expected %t", i, test.value.IsBulkString(), test.isBulk)
		}
		if test.value.IsInteger() != test.isInt {
			t.Errorf("Test %d: IsInteger() = %t, expected %t", i, test.value.IsInteger(), test.isInt)
		}
		if test.value.IsSimpleString() != test.isSimple {
			t.Errorf("Test %d: IsSimpleString() = %t, expected %t", i, test.value.IsSimpleString(), test.isSimple)
		}
		if test.value.IsError() != test.isError {
			t.Errorf("Test %d: IsError() = %t, expected %t", i, test.value.IsError(), test.isError)
		}
		if test.value.IsNull() != test.isNull {
			t.Errorf("Test %d: IsNull() = %t, expected %t", i, test.value.IsNull(), test.isNull)
		}
	}
}

func TestParser_InvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"invalid type", "?invalid\r\n"},
		{"no CRLF", "+OK"},
		{"incomplete bulk string", "$5\r\ntest"},
		{"invalid integer", ":abc\r\n"},
		{"invalid bulk length", "$abc\r\n"},
		{"invalid array length", "*abc\r\n"},
	}
	
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parser := NewParser(strings.NewReader(test.input))
			_, err := parser.Parse()
			if err == nil {
				t.Errorf("Expected error for invalid input %q", test.input)
			}
		})
	}
}

// Benchmark tests
func BenchmarkParser_SimpleString(b *testing.B) {
	input := "+OK\r\n"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser := NewParser(strings.NewReader(input))
		_, err := parser.Parse()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParser_BulkString(b *testing.B) {
	input := "$6\r\nfoobar\r\n"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser := NewParser(strings.NewReader(input))
		_, err := parser.Parse()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParser_Array(b *testing.B) {
	input := "*2\r\n$3\r\nGET\r\n$4\r\nkey1\r\n"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser := NewParser(strings.NewReader(input))
		_, err := parser.Parse()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFormatter_BulkString(b *testing.B) {
	formatter := NewFormatter()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		formatter.FormatBulkString("hello world test data")
	}
}
