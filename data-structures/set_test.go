package data_structures

import (
	"testing"
)

func TestSet_Length(t *testing.T) {
	s := NewSet([]int{})
	expect(t, s.Length(), 0)

	s2 := NewSet([]int{1, 1, 1, 2, 3, 3, 4, 4, 4, 4, 4, 5, 6, 6, 6})
	expect(t, s2.Length(), 6)
}

func TestSet_Add(t *testing.T) {
	s := NewSet([]int{1, 2, 3})

	s.Add(4)
	expect(t, s.Length(), 4)

	// Add already existing item
	s.Add(1)
	expect(t, s.Length(), 4)
}

func TestSet_Pop(t *testing.T) {
	s := NewSet([]int{1, 2})
	length := s.Length()

	_, err := s.Pop()
	if err != nil {
		panic(err)
	}
	expect(t, s.Length(), length-1)

	_, err = s.Pop()
	if err != nil {
		panic(err)
	}
	expect(t, s.Length(), length-2)

	expectPanic(t, func() { _, _ = s.Pop() })

}

func TestSet_ToArray(t *testing.T) {
	s := NewSet([]int{1, 2, 3, 4, 4, 4})
	arr := s.ToArray()
	expect(t, len(arr), 4)

	s2 := NewSet([]string{})
	arr2 := s2.ToArray()
	expect(t, len(arr2), 0)
}
