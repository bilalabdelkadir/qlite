package main

import "testing"

func TestExtractCommand(t *testing.T) {

	type testCase struct {
		input    string
		expected string
	}

	testCases := []testCase{
		{input: " ", expected: ""},
		{input: "select", expected: "SELECT"},
		{input: "insert into users values (1)", expected: "INSERT"},
		{input: "   update users set name='Bob'  ", expected: "UPDATE"},
		{input: "pRaGmA table_info(users)", expected: "PRAGMA"},
	}

	for _, testcase := range testCases {
		res := ExtractCommand(testcase.input)
		if res != testcase.expected {
			t.Errorf("for input %q: expected %q, got %q", testcase.input, testcase.expected, res)
		}
	}
}

func TestIsReadQuery(t *testing.T) {

	type testCase struct {
		input    string
		expected bool
	}

	testCases := []testCase{
		{input: "SELECT * FROM users", expected: true},
		{input: "INSERT INTO users VALUES (1)", expected: false},
		{input: "PRAGMA table_info(users)", expected: true},
		{input: "EXPLAIN QUERY PLAN SELECT *", expected: true},
		{input: "UPDATE users SET name='Bob'", expected: false},
		{input: "DELETE FROM users WHERE id=1", expected: false},
	}

	for _, testcase := range testCases {
		res := IsReadQuery(testcase.input)
		if res != testcase.expected {
			t.Errorf("for input %q: expected %t, got %t", testcase.input, testcase.expected, res)
		}
	}

}
